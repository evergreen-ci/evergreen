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
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
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
		go worker(workerCtx, jobs, r.queue, &r.wg)
		grip.Debugf("started worker %d of %d waiting for jobs", w, r.size)
	}

	return nil
}

// Close terminates all worker processes as soon as possible.
func (r *localWorkers) Close() {
	if r.canceler != nil {
		r.canceler()
		r.canceler = nil
		r.started = false
	}
	r.wg.Wait()
}
