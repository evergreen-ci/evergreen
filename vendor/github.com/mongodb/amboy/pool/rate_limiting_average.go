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
	"github.com/mongodb/grip/message"
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
func NewMovingAverageRateLimitedWorkers(size, targetNum int, period time.Duration, q amboy.Queue) (amboy.Runner, error) {
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
	}

	return p, nil
}

type ewmaRateLimiting struct {
	period   time.Duration
	target   int
	ewma     ewma.MovingAverage
	size     int
	queue    amboy.Queue
	canceler context.CancelFunc
	mutex    sync.Mutex
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

	jobs := startWorkerServer(ctx, p.queue, &p.wg)

	for w := 1; w <= p.size; w++ {
		go p.worker(ctx, jobs)
	}
	return nil
}

func (p *ewmaRateLimiting) worker(ctx context.Context, jobs <-chan amboy.Job) {
	var (
		err error
		job amboy.Job
	)

	p.wg.Add(1)
	defer p.wg.Done()
	defer func() {
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
			}
			// start a replacement worker.
			go p.worker(ctx, jobs)
		}
	}()

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			select {
			case <-ctx.Done():
				return
			case job := <-jobs:
				interval := p.runJob(ctx, job)

				timer.Reset(interval)
			}
		}
	}
}

func (p *ewmaRateLimiting) runJob(ctx context.Context, j amboy.Job) time.Duration {
	start := time.Now()
	ti := amboy.JobTimeInfo{
		Start: start,
	}

	j.Run()
	ti.End = time.Now()
	j.UpdateTimeInfo(ti)

	p.queue.Complete(ctx, j)
	duration := time.Since(start)

	interval := p.getNextTime(duration)
	r := message.Fields{
		"id":            j.ID(),
		"job_type":      j.Type().Name,
		"duration_secs": duration.Seconds(),
		"interval_secs": interval.Seconds(),
		"pool":          "rate limiting, moving average",
	}
	if err := j.Error(); err != nil {
		r["error"] = err.Error()
	}

	grip.Debug(r)

	return interval
}

func (p *ewmaRateLimiting) SetQueue(q amboy.Queue) error {
	if p.canceler != nil {
		return errors.New("cannot change queue on active runner")
	}

	p.queue = q
	return nil
}

func (p *ewmaRateLimiting) Close() {
	if p.canceler != nil {
		p.canceler()
	}
	grip.Debug("pool's context canceled, waiting for running jobs to complete")
	p.wg.Wait()
}
