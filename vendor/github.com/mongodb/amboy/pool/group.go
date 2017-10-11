/*
Grouped Pool

The MultiPool implementation is substantially similar to the Simple
Pool; however, one MultiPool instance can service jobs from multiple
Queues. In this case, the Runner *must* be attached to all of the
queues before workers are spawned and the queues begin dispatching
tasks.
*/
package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
)

// Group is a Runner implementation that can, potentially, run
// tasks from multiple queues at the same time.
type Group struct {
	size     int
	started  bool
	queues   []amboy.Queue
	wg       sync.WaitGroup
	mutex    sync.RWMutex
	canceler context.CancelFunc
}

type workUnit struct {
	j amboy.Job
	q amboy.Queue
}

// NewGroup start creates a new Runner instance with the
// specified number of worker processes capable of running jobs from
// multiple queues.
func NewGroup(numWorkers int) *Group {
	r := &Group{
		size: numWorkers,
	}

	return r
}

// SetQueue adds a new queue object to the Runner instance. There is
// no way to remove a amboy.Queue from a runner object, and no way to add a
// a amboy.Queue after starting to dispatch jobs.
func (r *Group) SetQueue(q amboy.Queue) error {
	if q.Started() {
		return errors.New("cannot add a started queue to a runner")
	}

	if r.Started() {
		return errors.New("cannot add a queue to a started runner")

	}

	r.queues = append(r.queues, q)
	return nil
}

// Started returns true if the runner's worker threads have started, and false otherwise.
func (r *Group) Started() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return r.started
}

func (r *Group) startMerger(ctx context.Context) <-chan *workUnit {
	// making this non-buffered so we don't have to wait as long for jobs to drain from the
	// channel in the event of a cancellation.
	work := make(chan *workUnit)

	go func() {
		// Make sure all queues are started...
		for _, queue := range r.queues {
			if !queue.Started() {
				err := queue.Start(ctx)
				if err != nil {
					grip.Error(err)
					continue
				}
			}
		}

		// start the background process for merging tasks from multiple queues into one
		// channel.
	mergerLoop:
		for {
			completed := 0
			for _, queue := range r.queues {
				stats := queue.Stats()
				if stats.Completed == stats.Total {
					completed++
					continue
				}

				job := queue.Next(ctx)
				if job == nil {
					continue
				}

				if job.Status().Completed {
					continue
				}

				task := &workUnit{
					j: job,
					q: queue,
				}

				select {
				case <-ctx.Done():
					break mergerLoop
				case work <- task:
					continue
				}
			}

			// we want to check if the context is canceled here too in case we hit the
			// continue conditions in the above loop, particularly on the next call.
			select {
			case <-ctx.Done():
				break mergerLoop
			default:
				if len(r.queues) == completed {
					break mergerLoop
				}
			}
		}
		close(work)
	}()

	return work
}

// Start initializes all worker process, and returns an error if the
// Runner has already started.
func (r *Group) Start(ctx context.Context) error {
	if r.Started() {
		// The group pool no-ops on successive Start
		// operations so that so that multiple queues can call
		// start.
		return nil
	}

	if len(r.queues) == 0 {
		return errors.New("group runner must have one queue configured")

	}

	ctx, cancel := context.WithCancel(ctx)
	r.canceler = cancel

	// Group is mostly similar to LocalWorker, but maintains a
	// background thread for each queue that puts Jobs onto a
	// channel, that the actual workers pull tasks from.
	work := r.startMerger(ctx)

	r.mutex.Lock()
	r.started = true
	r.mutex.Unlock()

	grip.Debugf("running %d workers", r.size)
	for w := 1; w <= r.size; w++ {
		r.wg.Add(1)
		name := fmt.Sprintf("worker-%d", w)

		go func(name string) {
			grip.Debugf("worker (%s) waiting for jobs", name)
		workLoop:
			for unit := range work {
				select {
				case <-ctx.Done():
					break workLoop
				default:
					unit.j.Run()
					unit.q.Complete(ctx, unit.j)
				}
			}
			grip.Debugf("worker (%s) exiting", name)
			r.wg.Done()
		}(name)
	}

	return nil
}

// Close cancels all pending workers and waits for the running
// processes to return.
func (r *Group) Close() {
	if r.canceler != nil {
		r.canceler()
	}

	r.wg.Wait()
}
