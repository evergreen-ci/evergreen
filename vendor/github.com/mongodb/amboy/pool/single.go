package pool

import (
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// single is an implementation of of the amboy.Runner interface
// that runs all tasks on one, and only one worker. Useful for
// testing the system with a different task executor.
type single struct {
	canceler context.CancelFunc
	queue    amboy.Queue
}

// NewSingle returns an amboy.Runner implementation with single-worker
// in the pool.
func NewSingle() amboy.Runner { return &single{} }

// Started returns true when the Runner has begun executing tasks. For
// LocalWorkers this means that workers are running.
func (r *single) Started() bool {
	return r.canceler != nil
}

// SetQueue allows callers to inject alternate amboy.Queue objects into
// constructed Runner objects. Returns an error if the Runner has
// started.
func (r *single) SetQueue(q amboy.Queue) error {
	if r.canceler != nil {
		return errors.New("cannot add new queue after starting a runner")
	}

	r.queue = q
	return nil
}

// Start takes a context and starts the internal worker and job
// processing thread. You can terminate the work of the Runner by
// canceling the context, or with the close method. Returns an error
// if the queue is not set. If the Runner is already running, Start is
// a no-op.
func (r *single) Start(ctx context.Context) error {
	if r.canceler != nil {
		return nil
	}

	if r.queue == nil {
		return errors.New("cannot start runner without a queue set")
	}

	workerCtx, cancel := context.WithCancel(ctx)
	r.canceler = cancel

	jobs := startWorkerServer(workerCtx, r.queue)

	go func() {
		worker(workerCtx, jobs, r.queue)
		grip.Info("worker process complete")
	}()

	grip.Info("started single queue worker")

	return nil
}

// Close terminates the work on the Runner. If a job is executing, the
// job will complete and the process will terminate before beginning a
// new job. If the queue has not started, Close is a no-op.
func (r *single) Close() {
	if r.canceler != nil {
		r.canceler()
	}
}
