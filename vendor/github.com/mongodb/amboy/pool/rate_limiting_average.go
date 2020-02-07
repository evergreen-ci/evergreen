package pool

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// NewMovingAverageRateLimitedWorkers returns a worker pool
// implementation that attempts to run a target number of tasks over a
// specified period to provide a more stable dispatching rate. It uses
// an exponentially weighted average of task time when determining the
// rate, which favors recent tasks over previous tasks.
//
// Returns an error if the size or target numbers are less than one
// and if the period is less than a millisecond.
func NewMovingAverageRateLimitedWorkers(size, targetNum int, period time.Duration, q amboy.Queue) (amboy.AbortableRunner, error) {
	errs := []string{}

	if targetNum <= 0 {
		errs = append(errs, "cannot specify a target number of tasks less than 1")
	}

	if size <= 0 {
		errs = append(errs, "cannot specify a pool size less than 1")
	}

	if period < time.Millisecond {
		errs = append(errs, "cannot specify a scheduling period interval less than a millisecond.")
	}

	if q == nil {
		errs = append(errs, "cannot specify a nil queue")
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	p := &ewmaRateLimiting{
		period: period,
		target: targetNum,
		size:   size,
		queue:  q,
		ewma:   ewma.NewMovingAverage(period.Minutes()),
		jobs:   make(map[string]context.CancelFunc),
	}

	return p, nil
}

type ewmaRateLimiting struct {
	period   time.Duration
	target   int
	ewma     ewma.MovingAverage
	size     int
	queue    amboy.Queue
	jobs     map[string]context.CancelFunc
	canceler context.CancelFunc
	mutex    sync.RWMutex
	wg       sync.WaitGroup
}

func (p *ewmaRateLimiting) getNextTime(dur time.Duration) time.Duration {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.ewma.Add(float64(dur))

	// find the average runtime of a recent job using or weighted moving average
	averageRuntime := time.Duration(math.Ceil(p.ewma.Value()))

	if averageRuntime == 0 {
		return time.Duration(0)
	}

	// find number of tasks per period, given the average runtime
	tasksPerPeriod := p.period / averageRuntime

	// the capacity of the pool is the size of the pool and the
	// target number of tasks
	capacity := time.Duration(p.target * p.size)

	// if the average runtime
	// of a task is such that the pool will run fewer than this
	// number of tasks, then no sleeping is necessary
	if tasksPerPeriod*capacity >= p.period {
		return time.Duration(0)
	}

	// if the average runtime times the capcity of the pool
	// (e.g. the theoretical max) is larger than the specified
	// period, no sleeping is required, because runtime is the
	// limiting factor.
	runtimePerPeriod := capacity * averageRuntime
	if runtimePerPeriod >= p.period {
		return time.Duration(0)
	}

	// therefore, there's excess time, which means we should sleep
	// for a fraction of that time before running the next job.
	//
	// we multiply by size here so that the interval/sleep time
	// scales as we add workers.
	excessTime := (p.period - runtimePerPeriod) * time.Duration(p.size)
	return (excessTime / time.Duration(p.target))
}

func (p *ewmaRateLimiting) Started() bool { return p.canceler != nil }
func (p *ewmaRateLimiting) Start(ctx context.Context) error {
	if p.canceler != nil {
		return nil
	}
	if p.queue == nil {
		return errors.New("runner must have an embedded queue")
	}

	ctx, p.canceler = context.WithCancel(ctx)

	for w := 1; w <= p.size; w++ {
		go p.worker(ctx)
	}
	return nil
}

func (p *ewmaRateLimiting) worker(bctx context.Context) {
	var (
		err    error
		job    amboy.Job
		ctx    context.Context
		cancel context.CancelFunc
	)

	p.mutex.Lock()
	p.wg.Add(1)
	p.mutex.Unlock()

	defer p.wg.Done()
	defer func() {
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
			}
			// start a replacement worker.
			go p.worker(bctx)
		}
		if cancel != nil {
			cancel()
		}
	}()

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-bctx.Done():
			return
		case <-timer.C:
			job := p.queue.Next(bctx)
			if job == nil {
				timer.Reset(jitterNilJobWait())
				continue
			}

			ctx, cancel = context.WithCancel(bctx)
			interval := p.runJob(ctx, job)
			cancel()

			timer.Reset(interval)
		}
	}
}

func (p *ewmaRateLimiting) addCanceler(id string, cancel context.CancelFunc) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.jobs[id] = cancel
}

func (p *ewmaRateLimiting) runJob(ctx context.Context, j amboy.Job) time.Duration {
	ti := j.TimeInfo()
	ti.Start = time.Now()

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	p.addCanceler(j.ID(), cancel)

	defer func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		delete(p.jobs, j.ID())
	}()

	executeJob(ctx, "rate-limited-average", j, p.queue)
	ti.End = time.Now()

	return ti.Duration()
}

func (p *ewmaRateLimiting) SetQueue(q amboy.Queue) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.canceler != nil {
		return errors.New("cannot change queue on active runner")
	}

	p.queue = q
	return nil
}

func (p *ewmaRateLimiting) Close(ctx context.Context) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for id, closer := range p.jobs {
		closer()
		delete(p.jobs, id)
	}

	if p.canceler == nil {
		return
	}

	p.canceler()
	p.canceler = nil
	grip.Debug("pool's context canceled, waiting for running jobs to complete")

	// because of the timer+2 contexts in the worker
	// implementation, we can end up returning earlier and because
	// pools are restartable, end up calling wait more than once,
	// which doesn't affect behavior but does cause this to panic in
	// tests
	defer func() { recover() }()
	wait := make(chan struct{})
	go func() {
		defer recovery.LogStackTraceAndContinue("waiting for close")
		defer close(wait)
		p.wg.Wait()
	}()

	select {
	case <-ctx.Done():
	case <-wait:
	}
}

func (p *ewmaRateLimiting) IsRunning(id string) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	_, ok := p.jobs[id]

	return ok
}

func (p *ewmaRateLimiting) RunningJobs() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	out := []string{}

	for id := range p.jobs {
		out = append(out, id)
	}

	return out
}

func (p *ewmaRateLimiting) Abort(ctx context.Context, id string) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	cancel, ok := p.jobs[id]
	if !ok {
		return errors.Errorf("job '%s' is not defined", id)
	}
	cancel()
	delete(p.jobs, id)

	job, ok := p.queue.Get(ctx, id)
	if !ok {
		return errors.Errorf("could not find '%s' in the queue", id)
	}

	p.queue.Complete(ctx, job)

	return nil
}

func (p *ewmaRateLimiting) AbortAll(ctx context.Context) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for id, cancel := range p.jobs {
		if ctx.Err() != nil {
			break
		}
		cancel()
		delete(p.jobs, id)
		job, ok := p.queue.Get(ctx, id)
		if !ok {
			continue
		}
		p.queue.Complete(ctx, job)
	}
}
