package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
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
type limitedSizeLocal struct {
	channel     chan amboy.Job
	toDelete    chan string
	capacity    int
	storage     map[string]amboy.Job
	scopes      ScopeManager
	dispatcher  Dispatcher
	lifetimeCtx context.Context

	deletedCount int
	staleCount   int
	id           string
	runner       amboy.Runner
	mu           sync.RWMutex
}

// NewLocalLimitedSize constructs a LocalLimitedSize queue instance
// with the specified number of workers and capacity.
func NewLocalLimitedSize(workers, capacity int) amboy.Queue {
	q := &limitedSizeLocal{
		capacity: capacity,
		storage:  make(map[string]amboy.Job),
		scopes:   NewLocalScopeManager(),
		id:       fmt.Sprintf("queue.local.unordered.fixed.%s", uuid.New().String()),
	}
	q.dispatcher = NewDispatcher(q)
	q.runner = pool.NewLocalWorkers(workers, q)
	return q
}

func (q *limitedSizeLocal) ID() string {
	return q.id
}

// Put adds a job to the queue, returning an error if the queue isn't
// opened, a task of that name exists has been completed (and is
// stored in the results storage,) or is pending, and finally if the
// queue is at capacity.
func (q *limitedSizeLocal) Put(ctx context.Context, j amboy.Job) error {
	if !q.Info().Started {
		return errors.Errorf("queue not open. could not add %s", j.ID())
	}

	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	if err := j.TimeInfo().Validate(); err != nil {
		return errors.Wrap(err, "invalid job timeinfo")
	}

	name := j.ID()

	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.storage[name]; ok {
		return amboy.NewDuplicateJobErrorf("cannot dispatch '%s', already complete", name)
	}

	if j.ShouldApplyScopesOnEnqueue() {
		if err := q.scopes.Acquire(name, j.Scopes()); err != nil {
			return errors.Wrapf(err, "applying scopes to job")
		}
	}

	select {
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "queue full, cannot add %s", name)
	case q.channel <- j:
		q.storage[name] = j
		return nil
	}
}

func (q *limitedSizeLocal) Save(ctx context.Context, j amboy.Job) error {
	if !q.Info().Started {
		return errors.Errorf("queue not open. could not add %s", j.ID())
	}

	name := j.ID()
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.storage[name]; !ok {
		return errors.Errorf("cannot save '%s', which is not tracked", name)
	}

	if err := q.scopes.Acquire(name, j.Scopes()); err != nil {
		return errors.Wrapf(err, "applying scopes to job")
	}

	q.storage[name] = j
	return nil
}

// Get returns a job, by name. This will include all tasks currently
// stored in the queue.
func (q *limitedSizeLocal) Get(ctx context.Context, name string) (amboy.Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	j, ok := q.storage[name]

	return j, ok
}

// Next returns the next pending job, and is used by amboy.Runner
// implementations to fetch work. This operation blocks until a job is
// available or the context is canceled.
func (q *limitedSizeLocal) Next(ctx context.Context) amboy.Job {
	misses := 0
	for {
		if misses > q.capacity {
			return nil
		}

		select {
		case job := <-q.channel:
			ti := job.TimeInfo()
			if ti.IsStale() {
				q.mu.Lock()
				delete(q.storage, job.ID())
				q.staleCount++
				q.mu.Unlock()

				grip.Notice(message.Fields{
					"state":    "stale",
					"job_id":   job.ID(),
					"job_type": job.Type().Name,
				})
				misses++
				continue
			}

			if !ti.IsDispatchable() {
				go q.requeue(job)
				misses++
				continue
			}

			if err := q.dispatcher.Dispatch(ctx, job); err != nil {
				go q.requeue(job)
				misses++
				continue
			}

			if err := q.scopes.Acquire(job.ID(), job.Scopes()); err != nil {
				q.dispatcher.Release(ctx, job)
				go q.requeue(job)
				misses++
				continue
			}

			return job
		case <-ctx.Done():
			return nil
		}
	}
}

func (q *limitedSizeLocal) requeue(job amboy.Job) {
	defer recovery.LogStackTraceAndContinue("re-queue waiting job", job.ID())
	select {
	case <-q.lifetimeCtx.Done():
	case q.channel <- job:
	}
}

func (q *limitedSizeLocal) Info() amboy.QueueInfo {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return amboy.QueueInfo{
		Started:     q.channel != nil,
		LockTimeout: amboy.LockTimeout,
	}
}

// Results is a generator of all completed tasks in the queue.
func (q *limitedSizeLocal) Results(ctx context.Context) <-chan amboy.Job {
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
func (q *limitedSizeLocal) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
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
func (q *limitedSizeLocal) Runner() amboy.Runner {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.runner
}

// SetRunner allows callers to, if the queue has not started, inject a
// different runner implementation.
func (q *limitedSizeLocal) SetRunner(r amboy.Runner) error {
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
func (q *limitedSizeLocal) Stats(ctx context.Context) amboy.QueueStats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	s := amboy.QueueStats{
		Total:     len(q.storage) + q.staleCount,
		Completed: len(q.toDelete) + q.deletedCount,
		Pending:   len(q.channel),
	}
	s.Running = s.Total - s.Completed - s.Pending
	return s
}

// Complete marks a job complete in the queue.
func (q *limitedSizeLocal) Complete(ctx context.Context, j amboy.Job) {
	if ctx.Err() != nil {
		return
	}
	q.dispatcher.Complete(ctx, j)

	q.mu.Lock()
	defer q.mu.Unlock()
	// save it
	status := j.Status()
	status.Completed = true
	status.InProgress = false
	status.ModificationTime = time.Now()
	status.ModificationCount += 1
	j.SetStatus(status)
	q.storage[j.ID()] = j

	if len(q.toDelete) == q.capacity-1 {
		delete(q.storage, <-q.toDelete)
		q.deletedCount++
	}

	grip.Alert(message.WrapError(
		q.scopes.Release(j.ID(), j.Scopes()),
		message.Fields{
			"id":     j.ID(),
			"scopes": j.Scopes(),
			"queue":  q.ID(),
			"op":     "releasing scope lock during completion",
		}))

	q.toDelete <- j.ID()
}

// Start starts the runner and initializes the pending task
// storage. Only produces an error if the underlying runner fails to
// start.
func (q *limitedSizeLocal) Start(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.channel != nil {
		return errors.New("cannot start a running queue")
	}

	q.lifetimeCtx = ctx
	q.toDelete = make(chan string, q.capacity)
	q.channel = make(chan amboy.Job, q.capacity)

	err := q.runner.Start(ctx)
	if err != nil {
		return err
	}

	grip.Info("job server running")

	return nil
}
