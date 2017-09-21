package queue

import (
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/pkg/errors"
	"github.com/mongodb/grip"
	"golang.org/x/net/context"
)

// LocalLimitedSize implements the amboy.Queue interface, and unlike
// other implementations, the size of the queue is limited for both
// incoming tasks and completed tasks. This makes it possible to use
// these queues in situations as parts of services and in
// longer-running contexts.
type LocalLimitedSize struct {
	channel  chan amboy.Job
	results  *driver.CappedResultStorage
	runner   amboy.Runner
	capacity int
	counters struct {
		queued    map[string]struct{}
		total     int
		started   int
		completed int
		sync.RWMutex
	}
}

// NewLocalLimitedSize constructs a LocalLimitedSize queue instance
// with the specified number of workers and capacity.
func NewLocalLimitedSize(workers, capacity int) *LocalLimitedSize {
	q := &LocalLimitedSize{
		results:  driver.NewCappedResultStorage(capacity),
		capacity: capacity,
	}
	q.runner = pool.NewLocalWorkers(workers, q)
	q.counters.queued = make(map[string]struct{})

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

	if _, ok := q.results.Get(name); ok {
		return errors.Errorf("cannot dispatch '%s', already complete", name)
	}

	q.counters.Lock()
	defer q.counters.Unlock()

	if _, ok := q.counters.queued[name]; ok {
		return errors.Errorf("cannot dispatch '%s', already in progress.", name)
	}

	select {
	case q.channel <- j:
		q.counters.total++
		q.counters.queued[j.ID()] = struct{}{}

		return nil
	default:
		return errors.Errorf("queue full, cannot add '%s'", name)
	}
}

// Get returns a job, by name, from the results storage. This does not
// retrieve pending tasks.
func (q *LocalLimitedSize) Get(name string) (amboy.Job, bool) {
	return q.results.Get(name)
}

// Next returns the next pending job, and is used by amboy.Runner
// implementations to fetch work. This operation blocks until a job is
// available or the context is canceled.
func (q *LocalLimitedSize) Next(ctx context.Context) amboy.Job {
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

// Started returns true if the queue is open and is processing jobs,
// and false otherwise.
func (q *LocalLimitedSize) Started() bool {
	return q.channel != nil
}

// Results is a generator of all completed tasks in the queue.
func (q *LocalLimitedSize) Results() <-chan amboy.Job {
	return q.results.Contents()
}

// Runner returns the Queue's embedded amboy.Runner instance.
func (q *LocalLimitedSize) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows callers to, if the queue has not started, inject a
// different runner implementation.
func (q *LocalLimitedSize) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot set runner on started queue")
	}

	q.runner = r

	return nil
}

// Stats returns information about the current state of jobs in the
// queue, and the amount of work completed.
func (q *LocalLimitedSize) Stats() amboy.QueueStats {
	q.counters.RLock()
	defer q.counters.RUnlock()

	return amboy.QueueStats{
		Total:     q.counters.total,
		Completed: q.counters.completed,
		Running:   q.counters.started - q.counters.completed,
		Pending:   len(q.channel),
	}
}

// Complete marks a job complete in the queue.
func (q *LocalLimitedSize) Complete(ctx context.Context, j amboy.Job) {
	name := j.ID()
	grip.Debugf("marking job (%s) as complete", name)
	q.counters.Lock()
	defer q.counters.Unlock()

	q.counters.completed++
	delete(q.counters.queued, name)
	q.results.Add(j)
}

// Start starts the runner and initializes the pending task
// storage. Only produces an error if the underlying runner fails to
// start.
func (q *LocalLimitedSize) Start(ctx context.Context) error {
	if q.channel != nil {
		return nil
	}

	q.channel = make(chan amboy.Job, q.capacity)

	err := q.runner.Start(ctx)

	if err != nil {
		return err
	}

	grip.Info("job server running")
	return nil
}
