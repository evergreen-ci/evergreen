/*
Local Workers Pool

The LocalWorkers is a simple worker pool implementation that spawns a
collection of (n) workers and dispatches jobs to worker threads, that
consume work items from the Queue's Next() method.
*/
package pool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
)

// NewLocalWorkers is a constructor for pool of worker processes that
// execute jobs from a queue locally, and takes arguments for
// the number of worker processes and a amboy.Queue object.
func NewLocalWorkers(numWorkers int, q amboy.Queue) amboy.Runner {
	r := &localWorkers{
		queue: q,
		size:  numWorkers,
	}

	if r.size <= 0 {
		grip.Infof("setting minimal pool size is 1, overriding setting of '%d'", r.size)
		r.size = 1
	}

	return r
}

// localWorkers is a very minimal implementation of a worker pool, and
// supports a configurable number of workers to process Job tasks.
type localWorkers struct {
	size     int
	started  bool
	queue    amboy.Queue
	canceler context.CancelFunc
	wg       sync.WaitGroup
}

// SetQueue allows callers to inject alternate amboy.Queue objects into
// constructed Runner objects. Returns an error if the Runner has
// started.
func (r *localWorkers) SetQueue(q amboy.Queue) error {
	if r.started {
		return errors.New("cannot add new queue after starting a runner")
	}

	r.queue = q
	return nil
}

// Started returns true when the Runner has begun executing tasks. For
// localWorkers this means that workers are running.
func (r *localWorkers) Started() bool {
	return r.started
}

func startWorkerServer(ctx context.Context, q amboy.Queue, wg *sync.WaitGroup) <-chan amboy.Job {
	output := make(chan amboy.Job)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				job := q.Next(ctx)
				if job == nil {
					continue
				}

				if job.Status().Completed {
					grip.Debugf("job '%s' was dispatched from the queue but was completed",
						job.ID())
					continue
				}

				output <- job
			}
		}
	}()

	return output
}

func worker(ctx context.Context, jobs <-chan amboy.Job, q amboy.Queue, wg *sync.WaitGroup) {
	var (
		err error
		job amboy.Job
	)

	wg.Add(1)
	defer wg.Done()
	defer func() {
		// if we hit a panic we want to add an error to the job;
		err = recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			if job != nil {
				job.AddError(err)
				q.Complete(ctx, job)
			}
			// start a replacement worker.
			go worker(ctx, jobs, q, wg)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case job = <-jobs:
			if job == nil {
				continue
			}

			ti := amboy.JobTimeInfo{
				Start: time.Now(),
			}

			job.UpdateTimeInfo(ti)
			job.Run()
			q.Complete(ctx, job)
			ti.End = time.Now()
			job.UpdateTimeInfo(ti)

			r := message.Fields{
				"job":           job.ID(),
				"job_type":      job.Type().Name,
				"duration_secs": ti.Duration().Seconds(),
				"queue_type":    fmt.Sprintf("%T", q),
			}
			if err := job.Error(); err != nil {
				r["error"] = err.Error()
			}
			grip.Debug(r)
		}
	}
}

// Start initializes all worker process, and returns an error if the
// Runner has already started.
func (r *localWorkers) Start(ctx context.Context) error {
	if r.started {
		return nil
	}

	if r.queue == nil {
		return errors.New("runner must have an embedded queue")
	}

	workerCtx, cancel := context.WithCancel(ctx)
	r.canceler = cancel
	jobs := startWorkerServer(workerCtx, r.queue, &r.wg)

	r.started = true
	grip.Debugf("running %d workers", r.size)

	for w := 1; w <= r.size; w++ {
		go func() {
			worker(workerCtx, jobs, r.queue, &r.wg)
		}()
		grip.Debugf("started worker %d of %d waiting for jobs", w, r.size)
	}

	return nil
}

// Close terminates all worker processes as soon as possible.
func (r *localWorkers) Close() {
	if r.canceler != nil {
		r.canceler()
	}
	r.wg.Wait()
}
