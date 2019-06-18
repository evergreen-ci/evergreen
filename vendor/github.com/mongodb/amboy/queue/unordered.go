/*
Local Unordered Queue

The unordered queue provides a basic, single-instance, amboy.Queue
that runs jobs locally in the context of the application with no
persistence layer. The unordered queue does not guarantee any
particular execution order, nor does it compute dependences between
jobs, but, as an implementation detail, dispatches jobs to workers in
a first-in-first-out (e.g. FIFO) model.

By default, LocalUnordered uses the amboy/pool.Workers implementation
of amboy.Runner interface.
*/
package queue

import (
	"context"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// LocalUnordered implements a local-only, channel based, queue
// interface, and it is a good prototype for testing, in addition to
// non-distributed workloads.
type unorderedLocal struct {
	started      bool
	numCompleted int
	numStarted   int
	channel      chan amboy.Job
	tasks        struct {
		m map[string]amboy.Job
		sync.RWMutex
	}

	runner amboy.Runner
}

// NewLocalUnordered is a constructor for a local queue that does not
// respect dependency information in dispatching queue jobs.
//
// All jobs are stored in memory and while there is a buffer of
// pending work, in general the number of buffered jobs is equal to
// twice the size of the worker pool, up to 64 jobs.
func NewLocalUnordered(workers int) amboy.Queue {
	bufferSize := workers * 2

	if bufferSize > 64 {
		bufferSize = 64
	}

	if bufferSize < 8 {
		bufferSize = 8
	}

	q := &unorderedLocal{
		channel: make(chan amboy.Job, bufferSize),
	}

	q.tasks.m = make(map[string]amboy.Job)

	grip.Debugln("queue buffer size:", bufferSize)

	r := pool.NewLocalWorkers(workers, q)
	q.runner = r

	return q
}

// Put adds a job to the amboy.Job Queue. Returns an error if the
// Queue has not yet started or if an amboy.Job with the
// same name (i.e. amboy.Job.ID()) exists.
func (q *unorderedLocal) Put(ctx context.Context, j amboy.Job) error {
	name := j.ID()

	if !q.started {
		return errors.Errorf("cannot add %s because queue has not started", name)
	}

	q.tasks.Lock()
	defer q.tasks.Unlock()

	if _, ok := q.tasks.m[name]; ok {
		return errors.Errorf("cannot add %s, because a job exists with that name", name)
	}

	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	select {
	case <-ctx.Done():
		return errors.Errorf("timed out adding %s to queue", name)
	case q.channel <- j:
		q.tasks.m[name] = j
		q.numStarted++
		grip.Debugf("added job (%s) to queue", j.ID())
		return nil
	}

}

// Runner returns the embedded task runner.
func (q *unorderedLocal) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows users to substitute alternate Runner
// implementations at run time. This method fails if the runner has
// started.
func (q *unorderedLocal) SetRunner(r amboy.Runner) error {
	if q.runner != nil && q.runner.Started() {
		return errors.New("cannot set a runner, current runner is running")
	}

	q.runner = r
	return r.SetQueue(q)
}

// Started returns true when the Queue has begun dispatching tasks to
// runners.
func (q *unorderedLocal) Started() bool {
	return q.started
}

// Start kicks off the background process that dispatches Jobs. Also
// starts the embedded runner, and errors if it cannot start. Should
// handle all errors from this method as fatal errors. If you call
// start on a queue that has been started, subsequent calls to Start()
// are a noop, and do not return an error.
func (q *unorderedLocal) Start(ctx context.Context) error {
	if q.started {
		return nil
	}

	err := q.runner.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting worker pool")
	}

	q.started = true

	grip.Info("job server running")
	return nil
}

// Next returns a job from the Queue. This call is non-blocking. If
// there are no pending jobs at the moment, then Next returns an
// error. If all jobs are complete, then Next also returns an error.
func (q *unorderedLocal) Next(ctx context.Context) amboy.Job {
	for {
		select {
		case <-ctx.Done():
			return nil
		case job := <-q.channel:
			return job
		}
	}
}

// Results provides an iterator of all "result objects," or completed
// amboy.Job objects. Does not wait for all results to be complete, and is
// closed when all results have been exhausted, even if there are more
// results pending. Other implementations may have different semantics
// for this method.
func (q *unorderedLocal) Results(ctx context.Context) <-chan amboy.Job {
	q.tasks.RLock()
	defer q.tasks.RUnlock()
	output := make(chan amboy.Job, q.numCompleted)

	go func() {
		q.tasks.RLock()
		defer q.tasks.RUnlock()
		defer close(output)
		for _, job := range q.tasks.m {
			if ctx.Err() != nil {
				return
			}

			if job.Status().Completed {
				output <- job
			}
		}
	}()

	return output
}

// JobStats returns JobStatusInfo objects for all jobs tracked by the
// queue, in no particular order.
func (q *unorderedLocal) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	out := make(chan amboy.JobStatusInfo)

	go func() {
		q.tasks.RLock()
		defer q.tasks.RUnlock()
		defer close(out)

		for _, job := range q.tasks.m {
			if ctx.Err() != nil {
				return
			}

			stat := job.Status()
			stat.ID = job.ID()
			select {
			case <-ctx.Done():
				return
			case out <- stat:
			}

		}

	}()

	return out
}

// Get takes a name and returns a completed job.
func (q *unorderedLocal) Get(ctx context.Context, name string) (amboy.Job, bool) {
	q.tasks.RLock()
	defer q.tasks.RUnlock()

	j, ok := q.tasks.m[name]

	return j, ok
}

// Stats returns a statistics object with data about the total number
// of jobs tracked by the queue.
func (q *unorderedLocal) Stats(ctx context.Context) amboy.QueueStats {
	s := amboy.QueueStats{}

	q.tasks.RLock()
	defer q.tasks.RUnlock()

	s.Completed = q.numCompleted
	s.Total = len(q.tasks.m)
	s.Pending = s.Total - s.Completed
	s.Running = q.numStarted - s.Completed
	return s
}

// Complete marks a job as complete, moving it from the in progress
// state to the completed state. This operation is asynchronous and non-blocking.
func (q *unorderedLocal) Complete(ctx context.Context, j amboy.Job) {
	if ctx.Err() != nil {
		return
	}
	go func() {
		grip.Debugf("marking job (%s) as complete", j.ID())
		q.tasks.Lock()
		defer q.tasks.Unlock()
		q.numCompleted++
	}()
}
