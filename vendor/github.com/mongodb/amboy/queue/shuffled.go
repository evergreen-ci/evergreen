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
	"fmt"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

// LocalShuffled provides a queue implementation that shuffles the
// order of jobs, relative the insertion order. Unlike
// some of the other local queue implementations that predate LocalShuffled
// (e.g. LocalUnordered,) there are no mutexes uses in the implementation.
type shuffledLocal struct {
	operations chan func(map[string]amboy.Job, map[string]amboy.Job, map[string]amboy.Job, *fixedStorage)
	capacity   int
	id         string
	starter    sync.Once
	runner     amboy.Runner
}

// NewShuffledLocal provides a queue implementation that shuffles the
// order of jobs, relative the insertion order.
func NewShuffledLocal(workers, capacity int) amboy.Queue {
	q := &shuffledLocal{
		capacity: capacity,
		id:       fmt.Sprintf("queue.local.unordered.shuffled.%s", uuid.NewV4().String()),
	}

	q.runner = pool.NewLocalWorkers(workers, q)
	return q
}

func (q *shuffledLocal) ID() string { return q.id }

// Start takes a context object and starts the embedded Runner instance
// and the queue's own background dispatching thread. Returns an error
// if there is no embedded runner, but is safe to call multiple times.
func (q *shuffledLocal) Start(ctx context.Context) error {
	if q.runner == nil {
		return errors.New("cannot start queue without a runner")
	}

	q.starter.Do(func() {
		q.operations = make(chan func(map[string]amboy.Job, map[string]amboy.Job, map[string]amboy.Job, *fixedStorage))
		go q.reactor(ctx)
		grip.Error(q.runner.Start(ctx))
		grip.Info("started shuffled job storage rector")
	})

	return nil
}

// reactor is the background dispatching process.
func (q *shuffledLocal) reactor(ctx context.Context) {
	defer recovery.LogStackTraceAndExit("shuffled amboy queue reactor")

	pending := make(map[string]amboy.Job)
	completed := make(map[string]amboy.Job)
	dispatched := make(map[string]amboy.Job)
	toDelete := newFixedStorage(q.capacity)

	for {
		select {
		case op := <-q.operations:
			op(pending, completed, dispatched, toDelete)
		case <-ctx.Done():
			grip.Info("shuffled storage reactor closing")
			return
		}
	}
}

// Put adds a job to the queue, and returns errors if the queue hasn't
// started or if a job with the same ID value already exists.
func (q *shuffledLocal) Put(ctx context.Context, j amboy.Job) error {
	id := j.ID()

	if !q.Started() {
		return errors.Errorf("cannot put job %s; queue not started", id)
	}

	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	if err := j.TimeInfo().Validate(); err != nil {
		return errors.Wrap(err, "invalid job timeinfo")
	}

	ret := make(chan error)
	op := func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {
		_, isPending := pending[id]
		_, isCompleted := completed[id]
		_, isDispatched := dispatched[id]

		if isPending || isCompleted || isDispatched {
			ret <- errors.Errorf("job '%s' already exists", id)
		}

		pending[id] = j

		close(ret)
	}

	select {
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	case q.operations <- op:
		return <-ret
	}
}

func (q *shuffledLocal) Save(ctx context.Context, j amboy.Job) error {
	id := j.ID()

	if !q.Started() {
		return errors.Errorf("cannot save job %s; queue not started", id)
	}

	ret := make(chan error)
	op := func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {
		defer close(ret)
		if _, ok := pending[id]; ok {
			pending[id] = j
			return
		}

		if _, ok := completed[id]; ok {
			completed[id] = j
			return
		}

		if _, ok := dispatched[id]; ok {
			dispatched[id] = j
			return
		}

		ret <- errors.Errorf("job '%s' does not exist", id)
	}

	select {
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	case q.operations <- op:
		return <-ret
	}
}

// Get returns a job based on the specified ID. Considers all pending,
// completed, and in progress jobs.
func (q *shuffledLocal) Get(ctx context.Context, name string) (amboy.Job, bool) {
	if !q.Started() {
		return nil, false
	}

	ret := make(chan amboy.Job)
	op := func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {
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

	select {
	case <-ctx.Done():
		return nil, false
	case q.operations <- op:
		job, ok := <-ret

		return job, ok
	}
}

// Results returns all completed jobs processed by the queue.
func (q *shuffledLocal) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)

	if !q.Started() {
		close(output)
		return output
	}

	q.operations <- func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {

		defer close(output)

		for _, job := range completed {
			select {
			case <-ctx.Done():
				return
			case output <- job:
				continue
			}
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

	q.operations <- func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {

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
func (q *shuffledLocal) Stats(ctx context.Context) amboy.QueueStats {
	if !q.Started() {
		return amboy.QueueStats{}
	}

	ret := make(chan amboy.QueueStats)
	q.operations <- func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {
		stat := amboy.QueueStats{
			Running:   len(dispatched),
			Pending:   len(pending),
			Completed: len(completed),
		}

		stat.Total = stat.Running + stat.Pending + stat.Completed

		ret <- stat
		close(ret)
	}

	select {
	case <-ctx.Done():
		return amboy.QueueStats{}
	case out := <-ret:
		return out
	}
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

	op := func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {
		defer close(ret)

		for id, j := range pending {
			if j.TimeInfo().IsStale() {
				delete(pending, j.ID())
				continue
			}

			select {
			case <-ctx.Done():
				return
			case ret <- j:
				dispatched[id] = j
				delete(pending, id)
				return
			}
		}
	}

	select {
	case <-ctx.Done():
		return nil
	case q.operations <- op:
		return <-ret
	}
}

// Complete marks a job as complete in the internal representation. If
// the context is canceled after calling Complete but before it
// executes, no change occurs.
func (q *shuffledLocal) Complete(ctx context.Context, j amboy.Job) {
	if ctx.Err() != nil {
		return
	}

	op := func(
		pending map[string]amboy.Job,
		completed map[string]amboy.Job,
		dispatched map[string]amboy.Job,
		toDelete *fixedStorage,
	) {

		id := j.ID()

		completed[id] = j
		delete(dispatched, id)
		toDelete.Push(id)

		if num := toDelete.Oversize(); num > 0 {
			for i := 0; i < num; i++ {
				delete(completed, toDelete.Pop())
			}
		}
	}

	select {
	case <-ctx.Done():
	case q.operations <- op:
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
