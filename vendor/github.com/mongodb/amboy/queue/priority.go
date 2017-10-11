package queue

import (
	"context"
	"errors"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/grip"
)

// LocalPriorityQueue is an amboy.Queue implementation that dispatches
// jobs in priority order, using the Priority method of the Job
// interface to determine priority. These queues do not have shared
// storage.
type LocalPriorityQueue struct {
	storage  *driver.PriorityStorage
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
func NewLocalPriorityQueue(workers int) *LocalPriorityQueue {
	q := &LocalPriorityQueue{
		storage: driver.NewPriorityStorage(),
	}

	q.runner = pool.NewLocalWorkers(workers, q)
	return q
}

// Put adds a job to the priority queue. If the Job already exists,
// this operation updates it in the queue, potentially reordering the
// queue accordingly.
func (q *LocalPriorityQueue) Put(j amboy.Job) error {
	q.storage.Push(j)

	return nil
}

// Get takes the name of a job and returns the job from the queue that
// matches that ID. Use the second return value to check if a job
// object with that ID exists in the queue.e
func (q *LocalPriorityQueue) Get(name string) (amboy.Job, bool) {
	return q.storage.Get(name)
}

// Next returns a job for processing the queue. This may be a nil job
// if the context is canceled. Otherwise, this operation blocks until
// a job is available for dispatching.
func (q *LocalPriorityQueue) Next(ctx context.Context) amboy.Job {
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
func (q *LocalPriorityQueue) Started() bool {
	return q.channel != nil
}

// Results is a generator of all jobs that report as "Completed" in
// the queue.
func (q *LocalPriorityQueue) Results() <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func() {
		for job := range q.storage.Contents() {
			if job.Status().Completed {
				output <- job
			}
		}
		close(output)
	}()

	return output
}

// Runner returns the embedded runner instance, which provides and
// manages the worker processes.
func (q *LocalPriorityQueue) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows users to override the default embedded runner. This
// is *only* possible if the queue has not started processing jobs. If
// you attempt to set the runner after the queue has started the
// operation returns an error and has no effect.
func (q *LocalPriorityQueue) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot set runner after queue is started")
	}

	q.runner = r

	return nil
}

// Stats returns an amboy.QueueStats object that reflects the queue's
// current state.
func (q *LocalPriorityQueue) Stats() amboy.QueueStats {
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
func (q *LocalPriorityQueue) Complete(ctx context.Context, j amboy.Job) {
	grip.Debugf("marking job (%s) as complete", j.ID())
	q.counters.Lock()
	defer q.counters.Unlock()

	q.counters.completed++
}

// Start begins the work of the queue. It is a noop, without error, to
// start a queue that's been started, but the operation can error if
// there were problems starting the underlying runner instance. All
// resources are released when the context is canceled.
func (q *LocalPriorityQueue) Start(ctx context.Context) error {
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
