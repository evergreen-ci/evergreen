package queue

import (
	"context"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// LocalLimitedSize implements the amboy.Queue interface, and unlike
// other implementations, the size of the queue is limited for both
// incoming tasks and completed tasks. This makes it possible to use
// these queues in situations as parts of services and in
// longer-running contexts.
//
// Specify a capacity when constructing the queue; the queue will
// store no more than 2x the number specified, and no more the
// specified capacity of completed jobs.
type LocalLimitedSize struct {
	channel  chan amboy.Job
	toDelete chan string
	capacity int
	storage  map[string]amboy.Job

	runner amboy.Runner
	mu     sync.RWMutex
}

// NewLocalLimitedSize constructs a LocalLimitedSize queue instance
// with the specified number of workers and capacity.
func NewLocalLimitedSize(workers, capacity int) *LocalLimitedSize {
	q := &LocalLimitedSize{
		capacity: capacity,
		storage:  make(map[string]amboy.Job),
	}
	q.runner = pool.NewLocalWorkers(workers, q)
	return q
}

// Put adds a job to the queue, returning an error if the queue isn't
// opened, a task of that name exists has been completed (and is
// stored in the results storage,) or is pending, and finally if the
// queue is at capacity.
func (q *LocalLimitedSize) Put(j amboy.Job) error {
	if !q.Started() {
		return errors.Errorf("queue not open. could not add %s", j.ID())
	}

	name := j.ID()

	q.mu.RLock()
	if _, ok := q.storage[name]; ok {
		q.mu.RUnlock()
		return errors.Errorf("cannot dispatch '%s', already complete", name)
	}
	q.mu.RUnlock()

	select {
	case q.channel <- j:
		q.mu.Lock()
		defer q.mu.Unlock()
		q.storage[name] = j

		return nil
	default:
		return errors.Errorf("queue full, cannot add '%s'", name)
	}
}

// Get returns a job, by name. This will include all tasks currently
// stored in the queue.
func (q *LocalLimitedSize) Get(name string) (amboy.Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	j, ok := q.storage[name]

	return j, ok
}

// Next returns the next pending job, and is used by amboy.Runner
// implementations to fetch work. This operation blocks until a job is
// available or the context is canceled.
func (q *LocalLimitedSize) Next(ctx context.Context) amboy.Job {
	select {
	case job := <-q.channel:
		return job
	case <-ctx.Done():
		return nil
	}
}

// Started returns true if the queue is open and is processing jobs,
// and false otherwise.
func (q *LocalLimitedSize) Started() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.channel != nil
}

// Results is a generator of all completed tasks in the queue.
func (q *LocalLimitedSize) Results(ctx context.Context) <-chan amboy.Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	newCompleted := make(chan string, q.capacity)
	out := make(chan amboy.Job, len(q.toDelete))
	close(q.toDelete)
	for name := range q.toDelete {
		j := q.storage[name]
		newCompleted <- name
		out <- j
	}
	close(out)
	q.toDelete = newCompleted

	return out
}

// JobStats returns an iterator for job status documents for all jobs
// in the queue. For this queue implementation *queued* jobs are returned
// first.
func (q *LocalLimitedSize) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	q.mu.RLock()
	defer q.mu.RUnlock()

	out := make(chan amboy.JobStatusInfo, len(q.storage))
	for name, job := range q.storage {
		stat := job.Status()
		stat.ID = name
		out <- stat
	}
	close(out)

	return out
}

// Runner returns the Queue's embedded amboy.Runner instance.
func (q *LocalLimitedSize) Runner() amboy.Runner {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.runner
}

// SetRunner allows callers to, if the queue has not started, inject a
// different runner implementation.
func (q *LocalLimitedSize) SetRunner(r amboy.Runner) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.channel != nil {
		return errors.New("cannot set runner on started queue")
	}

	q.runner = r

	return nil
}

// Stats returns information about the current state of jobs in the
// queue, and the amount of work completed.
func (q *LocalLimitedSize) Stats() amboy.QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	s := amboy.QueueStats{
		Total:     len(q.storage),
		Completed: len(q.toDelete),
		Pending:   len(q.channel),
	}
	s.Running = s.Total - s.Completed - s.Pending
	return s
}

// Complete marks a job complete in the queue.
func (q *LocalLimitedSize) Complete(ctx context.Context, j amboy.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// save it
	q.storage[j.ID()] = j

	if len(q.toDelete) == q.capacity-1 {
		delete(q.storage, <-q.toDelete)
	}
	q.toDelete <- j.ID()
}

// Start starts the runner and initializes the pending task
// storage. Only produces an error if the underlying runner fails to
// start.
func (q *LocalLimitedSize) Start(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.channel != nil {
		return errors.New("cannot start a running queue")
	}

	q.toDelete = make(chan string, q.capacity)
	q.channel = make(chan amboy.Job, q.capacity)

	err := q.runner.Start(ctx)
	if err != nil {
		return err
	}

	grip.Info("job server running")

	return nil
}
