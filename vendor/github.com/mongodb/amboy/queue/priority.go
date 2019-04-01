package queue

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
)

// LocalPriorityQueue is an amboy.Queue implementation that dispatches
// jobs in priority order, using the Priority method of the Job
// interface to determine priority. These queues do not have shared
// storage.
type priorityLocalQueue struct {
	storage  *priorityStorage
	fixed    *fixedStorage
	channel  chan amboy.Job
	runner   amboy.Runner
	counters struct {
		started   int
		completed int
		sync.RWMutex
	}
}

// NewLocalPriorityQueue constructs a new priority queue instance and
// initializes a local worker queue with the specified number of
// worker processes.
func NewLocalPriorityQueue(workers, capacity int) amboy.Queue {
	q := &priorityLocalQueue{
		storage: makePriorityStorage(),
		fixed:   newFixedStorage(capacity),
	}

	q.runner = pool.NewLocalWorkers(workers, q)
	return q
}

// Put adds a job to the priority queue. If the Job already exists,
// this operation updates it in the queue, potentially reordering the
// queue accordingly.
func (q *priorityLocalQueue) Put(j amboy.Job) error {
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	return q.storage.Insert(j)
}

// Get takes the name of a job and returns the job from the queue that
// matches that ID. Use the second return value to check if a job
// object with that ID exists in the queue.e
func (q *priorityLocalQueue) Get(name string) (amboy.Job, bool) {
	return q.storage.Get(name)
}

// Next returns a job for processing the queue. This may be a nil job
// if the context is canceled. Otherwise, this operation blocks until
// a job is available for dispatching.
func (q *priorityLocalQueue) Next(ctx context.Context) amboy.Job {
	select {
	case <-ctx.Done():
		return nil
	case job := <-q.channel:
		q.counters.Lock()
		defer q.counters.Unlock()
		q.counters.started++
		return job
	}
}

// Started reports if the queue has begun processing work.
func (q *priorityLocalQueue) Started() bool {
	return q.channel != nil
}

// Results is a generator of all jobs that report as "Completed" in
// the queue.
func (q *priorityLocalQueue) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func() {
		defer close(output)
		for job := range q.storage.Contents() {
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

// JobStats returns a job status for every job stored in the
// queue. Does not include currently in progress tasks.
func (q *priorityLocalQueue) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	out := make(chan amboy.JobStatusInfo)

	go func() {
		defer close(out)
		for job := range q.storage.Contents() {
			if ctx.Err() != nil {
				return
			}
			stat := job.Status()
			stat.ID = job.ID()
			out <- stat
		}
	}()

	return out
}

// Runner returns the embedded runner instance, which provides and
// manages the worker processes.
func (q *priorityLocalQueue) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows users to override the default embedded runner. This
// is *only* possible if the queue has not started processing jobs. If
// you attempt to set the runner after the queue has started the
// operation returns an error and has no effect.
func (q *priorityLocalQueue) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot set runner after queue is started")
	}

	q.runner = r

	return nil
}

// Stats returns an amboy.QueueStats object that reflects the queue's
// current state.
func (q *priorityLocalQueue) Stats() amboy.QueueStats {
	stats := amboy.QueueStats{
		Total:   q.storage.Size(),
		Pending: q.storage.Pending(),
	}

	q.counters.RLock()
	defer q.counters.RUnlock()

	stats.Completed = q.counters.completed
	stats.Running = q.counters.started - q.counters.completed

	return stats
}

// Complete marks a job complete. The operation is asynchronous in
// this implementation.
func (q *priorityLocalQueue) Complete(ctx context.Context, j amboy.Job) {
	id := j.ID()
	grip.Debugf("marking job (%s) as complete", id)
	q.counters.Lock()
	defer q.counters.Unlock()

	q.fixed.Push(id)

	if num := q.fixed.Oversize(); num > 0 {
		for i := 0; i < num; i++ {
			q.storage.Remove(q.fixed.Pop())
		}
	}

	q.counters.completed++
}

// Start begins the work of the queue. It is a noop, without error, to
// start a queue that's been started, but the operation can error if
// there were problems starting the underlying runner instance. All
// resources are released when the context is canceled.
func (q *priorityLocalQueue) Start(ctx context.Context) error {
	if q.channel != nil {
		return nil
	}

	q.channel = make(chan amboy.Job)

	go q.storage.JobServer(ctx, q.channel)

	err := q.runner.Start(ctx)
	if err != nil {
		return err
	}

	grip.Info("job server running")

	return nil
}
