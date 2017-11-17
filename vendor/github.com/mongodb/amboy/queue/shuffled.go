/*
Local Shuffled Queue

The shuffled queue is functionally similar to the LocalUnordered Queue
(which is, in fact, a FIFO queue as a result of its implementation);
however, the shuffled queue dispatches tasks randomized, using the
properties of Go's map type, which is not dependent on insertion
order.

Additionally this implementation does not using locking, which may
improve performance for some workloads. Intentionally, the
implementation retains pointers to all completed tasks, and does not
cap the number of pending tasks.
*/
package queue

import (
	"context"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// LocalShuffled provides a queue implementation that shuffles the
// order of jobs, relative the insertion order. Unlike
// some of the other local queue implementations that predate LocalShuffled
// (e.g. LocalUnordered,) there are no mutexes uses in the implementation.
type shuffledLocal struct {
	operations chan func(map[string]amboy.Job, map[string]amboy.Job, map[string]amboy.Job)
	starter    sync.Once
	runner     amboy.Runner
}

// NewShuffledLocal provides a queue implementation that shuffles the
// order of jobs, relative the insertion order.
func NewShuffledLocal(workers int) amboy.Queue {
	q := &shuffledLocal{}
	q.runner = pool.NewLocalWorkers(workers, q)

	return q
}

// Start takes a context object and starts the embedded Runner instance
// and the queue's own background dispatching thread. Returns an error
// if there is no embedded runner, but is safe to call multiple times.
func (q *shuffledLocal) Start(ctx context.Context) error {
	if q.runner == nil {
		return errors.New("cannot start queue without a runner")
	}

	q.starter.Do(func() {
		q.operations = make(chan func(map[string]amboy.Job, map[string]amboy.Job, map[string]amboy.Job))
		go q.reactor(ctx)
		grip.CatchError(q.runner.Start(ctx))
		grip.Info("started shuffled job storage rector")
	})

	return nil
}

// reactor is the background dispatching process.
func (q *shuffledLocal) reactor(ctx context.Context) {
	pending := make(map[string]amboy.Job)
	completed := make(map[string]amboy.Job)
	dispatched := make(map[string]amboy.Job)

	for {
		select {
		case op := <-q.operations:
			op(pending, completed, dispatched)
		case <-ctx.Done():
			grip.Info("shuffled storage reactor closing")
			return
		}
	}
}

// Put adds a job to the queue, and returns errors if the queue hasn't
// started or if a job with the same ID value already exists.
func (q *shuffledLocal) Put(j amboy.Job) error {
	id := j.ID()

	if !q.Started() {
		return errors.Errorf("cannot put job %s; queue not started", id)
	}

	ret := make(chan error)
	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		_, isPending := pending[id]
		_, isCompleted := completed[id]
		_, isDispatched := dispatched[id]
		if isPending || isCompleted || isDispatched {
			ret <- errors.Errorf("job '%s' already exists", id)
		}

		pending[id] = j

		close(ret)
	}

	return <-ret
}

// Get returns a job based on the specified ID. Considers all pending,
// completed, and in progress jobs.
func (q *shuffledLocal) Get(name string) (amboy.Job, bool) {
	if !q.Started() {
		return nil, false
	}

	ret := make(chan amboy.Job)
	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		defer close(ret)

		if job, ok := pending[name]; ok {
			ret <- job
			return
		}

		if job, ok := completed[name]; ok {
			ret <- job
			return
		}

		if job, ok := dispatched[name]; ok {
			ret <- job
			return
		}
	}

	job, ok := <-ret

	return job, ok
}

// Results returns all completed jobs processed by the queue.
func (q *shuffledLocal) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)

	if !q.Started() {
		close(output)
		return output
	}

	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		defer close(output)

		for _, job := range completed {
			if ctx.Err() != nil {
				return
			}

			output <- job
		}
	}

	return output
}

// JobStats returns JobStatusInfo objects for all jobs tracked by the
// queue. The operation returns jobs first that have been dispatched
// (e.g. currently working,) then pending (queued for dispatch,) and
// finally completed.
func (q *shuffledLocal) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	out := make(chan amboy.JobStatusInfo)

	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		defer close(out)
		for _, j := range dispatched {
			if ctx.Err() != nil {
				return
			}
			stat := j.Status()
			stat.ID = j.ID()
			out <- stat
		}
		for _, j := range pending {
			if ctx.Err() != nil {
				return
			}
			stat := j.Status()
			stat.ID = j.ID()
			out <- stat
		}
		for _, j := range completed {
			if ctx.Err() != nil {
				return
			}
			stat := j.Status()
			stat.ID = j.ID()
			out <- stat
		}
	}

	return out
}

// Stats returns a standard report on the number of pending, running,
// and completed jobs processed by the queue.
func (q *shuffledLocal) Stats() amboy.QueueStats {
	if !q.Started() {
		return amboy.QueueStats{}
	}

	ret := make(chan amboy.QueueStats)
	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		stat := amboy.QueueStats{
			Running:   len(dispatched),
			Pending:   len(pending),
			Completed: len(completed),
		}

		stat.Total = stat.Running + stat.Pending + stat.Completed

		ret <- stat
		close(ret)
	}
	return <-ret
}

// Started returns true after the queue has started processing work,
// and false otherwise. When the queue has terminated (as a result of
// the starting context's cancellation.
func (q *shuffledLocal) Started() bool {
	return q.operations != nil
}

// Next returns a new pending job, and is used by the Runner interface
// to fetch new jobs. This method returns a nil job object is there are
// no pending jobs.
func (q *shuffledLocal) Next(ctx context.Context) amboy.Job {
	ret := make(chan amboy.Job)
	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		if ctx.Err() != nil {
			close(ret)
			return
		}

		if len(pending) == 0 {
			close(ret)
			return
		}

		for id, j := range pending {
			ret <- j
			dispatched[id] = j
			delete(pending, id)
			close(ret)
			return
		}
	}

	return <-ret
}

// Complete marks a job as complete in the internal representation. If
// the context is canceled after calling Complete but before it
// executes, no change occurs.
func (q *shuffledLocal) Complete(ctx context.Context, j amboy.Job) {
	q.operations <- func(pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job) {

		id := j.ID()

		if ctx.Err() != nil {
			grip.Noticef("did not complete %s job, because operation "+
				"was canceled.", id)
			return
		}

		completed[id] = j
		delete(dispatched, id)
	}
}

// SetRunner modifies the embedded amboy.Runner instance, and return an
// error if the current runner has started.
func (q *shuffledLocal) SetRunner(r amboy.Runner) error {
	if q.runner != nil && q.runner.Started() {
		return errors.New("cannot set a runner, current runner is running")
	}

	q.runner = r
	return r.SetQueue(q)
}

// Runner returns the embedded runner.
func (q *shuffledLocal) Runner() amboy.Runner {
	return q.runner
}
