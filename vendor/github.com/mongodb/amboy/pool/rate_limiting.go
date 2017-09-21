/*
Rate Limiting Pools

Amboy includes two rate limiting pools, to control the flow of tasks
processed by the queue. The "simple" implementation sleeps for a
configurable interval in-between each task, while the averaged tool,
uses an exponential weighted average and a targeted number of tasks to
complete over an interval to achieve a reasonable flow of tasks
through the runner.
*/
package pool

import (
	"errors"
	"strings"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// NewSimpleRateLimitedWorkers returns a worker pool that sleeps for
// the specified interval after completing each task. After that
// interval, the runner will run the next available task as soon as its ready.
//
// The constructor returns an error if the size (number of workers) is
// less than 1 or the interval is less than a millisecond.
func NewSimpleRateLimitedWorkers(size int, sleepInterval time.Duration, q amboy.Queue) (amboy.Runner, error) {
	errs := []string{}

	if size <= 0 {
		errs = append(errs, "cannot specify a pool size less than 1")
	}

	if sleepInterval < time.Millisecond {
		errs = append(errs, "cannot specify a sleep interval less than a millisecond.")
	}

	if q == nil {
		errs = append(errs, "cannot specify a nil queue")
	}

	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	p := &simpleRateLimited{
		size:     size,
		interval: sleepInterval,
		queue:    q,
	}

	return p, nil
}

type simpleRateLimited struct {
	size     int
	interval time.Duration
	queue    amboy.Queue
	canceler context.CancelFunc
}

func (p *simpleRateLimited) Started() bool { return p.canceler != nil }
func (p *simpleRateLimited) Start(ctx context.Context) error {
	if p.canceler != nil {
		return nil
	}
	if p.queue == nil {
		return errors.New("runner must have an embedded queue")
	}

	ctx, p.canceler = context.WithCancel(ctx)

	jobs := startWorkerServer(ctx, p.queue)

	// start some threads
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
						job.Run()
						p.queue.Complete(ctx, job)
						timer.Reset(p.interval)
					}
				}
			}
		}()
		grip.Debugf("started rate limited worker %d of %d ", w, p.size)
	}
	return nil
}

func (p *simpleRateLimited) SetQueue(q amboy.Queue) error {
	if p.canceler != nil {
		return errors.New("cannot change queue on active runner")
	}

	p.queue = q
	return nil
}

func (p *simpleRateLimited) Close() {
	if p.canceler != nil {
		p.canceler()
	}
}
