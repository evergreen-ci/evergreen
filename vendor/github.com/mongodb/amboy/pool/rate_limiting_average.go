package pool

import (
	"math"
	"strings"
	"sync"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
		ewma:   ewma.NewMovingAverage(),
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
}

func (p *ewmaRateLimiting) getNextTime(dur time.Duration) time.Duration {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.ewma.Add(float64(dur))

	adjustedRuntime := time.Duration(math.Ceil(p.ewma.Value())) / time.Duration(p.size)
	runtimeOfTargetNumber := adjustedRuntime * time.Duration(p.target)

	// if the expected runtime of the target number of tasks
	// (adjisted for the size of the pool) is less than the stated
	// period, return the difference between the total expected
	// runtime of the tasks (adjusted) and the period, divided by
	// the target number of tasks

	if runtimeOfTargetNumber < p.period {
		workerRestingTime := p.period - runtimeOfTargetNumber
		return workerRestingTime / time.Duration(p.target)
	}

	// if the expected runtime of the target number of tasks is
	// greater than or equal to the stated period, return 0
	return time.Duration(0)
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

	jobs := startWorkerServer(ctx, p.queue)

	for w := 1; w <= p.size; w++ {
		go func() {
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
		}()
	}
	return nil
}

func (p *ewmaRateLimiting) runJob(ctx context.Context, j amboy.Job) time.Duration {
	start := time.Now()
	j.Run()
	p.queue.Complete(ctx, j)
	duration := time.Since(start)

	interval := p.getNextTime(duration)

	grip.Debugf("task %s completed in %s, next job in %s",
		j.ID(), duration, interval)

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
}
