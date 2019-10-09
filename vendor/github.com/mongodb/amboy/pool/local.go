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
	wg       sync.WaitGroup
	canceler context.CancelFunc
	queue    amboy.Queue
	mu       sync.RWMutex
}

// SetQueue allows callers to inject alternate amboy.Queue objects into
// constructed Runner objects. Returns an error if the Runner has
// started.
func (r *localWorkers) SetQueue(q amboy.Queue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errors.New("cannot add new queue after starting a runner")
	}

	r.queue = q
	return nil
}

// Started returns true when the Runner has begun executing tasks. For
// localWorkers this means that workers are running.
func (r *localWorkers) Started() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started
}

// Start initializes all worker process, and returns an error if the
// Runner has already started.
func (r *localWorkers) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		return nil
	}

	if r.queue == nil {
		return errors.New("runner must have an embedded queue")
	}

	workerCtx, cancel := context.WithCancel(ctx)
	r.canceler = cancel

	for w := 1; w <= r.size; w++ {
		go worker(workerCtx, "local", r.queue, &r.wg)
		grip.Debugf("started worker %d of %d waiting for jobs", w, r.size)
	}

	r.started = true
	grip.Debugf("running %d workers", r.size)

	return nil
}

// Close terminates all worker processes as soon as possible.
func (r *localWorkers) Close(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.canceler != nil {
		r.canceler()
		r.canceler = nil
		r.started = false
	}

	wait := make(chan struct{})
	go func() {
		defer recovery.LogStackTraceAndContinue("waiting for close")
		defer close(wait)
		r.wg.Wait()
	}()

	select {
	case <-ctx.Done():
	case <-wait:
	}
}
