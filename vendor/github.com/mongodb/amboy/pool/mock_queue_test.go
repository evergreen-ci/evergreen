package pool

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/pkg/errors"
)

type QueueTester struct {
	started     bool
	pool        amboy.Runner
	id          string
	numComplete int
	toProcess   chan amboy.Job
	storage     map[string]amboy.Job

	mutex sync.RWMutex
}

func NewQueueTester(p amboy.Runner) *QueueTester {
	q := NewQueueTesterInstance()
	_ = p.SetQueue(q)
	q.pool = p

	return q
}

// Separate constructor for the object so we can avoid the side
// effects of the extra SetQueue for tests where that doesn't make
// sense.
func NewQueueTesterInstance() *QueueTester {
	return &QueueTester{
		toProcess: make(chan amboy.Job, 101),
		storage:   make(map[string]amboy.Job),
		id:        uuid.New().String(),
	}
}

func (q *QueueTester) Put(ctx context.Context, j amboy.Job) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.toProcess <- j:
		q.mutex.Lock()
		defer q.mutex.Unlock()

		q.storage[j.ID()] = j
		return nil
	}
}

func (q *QueueTester) Save(ctx context.Context, j amboy.Job) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	id := j.ID()
	if _, ok := q.storage[id]; !ok {
		return nil
	}
	q.storage[id] = j
	return nil
}

func (q *QueueTester) ID() string { return fmt.Sprintf("queue.tester.%s", q.id) }

func (q *QueueTester) Get(ctx context.Context, name string) (amboy.Job, bool) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	job, ok := q.storage[name]
	return job, ok
}

func (q *QueueTester) Started() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return q.started
}

func (q *QueueTester) Complete(ctx context.Context, j amboy.Job) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.numComplete++
}

func (q *QueueTester) Stats(ctx context.Context) amboy.QueueStats {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return amboy.QueueStats{
		Running:   len(q.storage) - len(q.toProcess),
		Completed: q.numComplete,
		Pending:   len(q.toProcess),
		Total:     len(q.storage),
	}
}

func (q *QueueTester) Runner() amboy.Runner {
	return q.pool
}

func (q *QueueTester) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot set runner in a started pool")
	}
	q.pool = r
	return nil
}

func (q *QueueTester) Next(ctx context.Context) amboy.Job {
	select {
	case <-ctx.Done():
		return nil
	case job := <-q.toProcess:
		return job
	}
}

func (q *QueueTester) Start(ctx context.Context) error {
	if q.Started() {
		return nil
	}

	err := q.pool.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting worker pool")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	q.started = true
	return nil
}

func (q *QueueTester) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func() {
		defer close(output)
		for _, job := range q.storage {
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

func (q *QueueTester) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	output := make(chan amboy.JobStatusInfo)
	go func() {
		defer close(output)
		for _, job := range q.storage {
			if ctx.Err() != nil {
				return

			}
			status := job.Status()
			status.ID = job.ID()
			output <- status
		}
	}()

	return output
}

type jobThatPanics struct {
	job.Base
}

func (j *jobThatPanics) Run(_ context.Context) {
	defer j.MarkComplete()

	panic("panic err")
}

func jobsChanWithPanicingJobs(ctx context.Context, num int) chan amboy.Job {
	out := make(chan amboy.Job)

	go func() {
		defer close(out) // nolint
		count := 0
		for {
			if count >= num {
				return
			}

			select {
			case <-ctx.Done():
				return
			case out <- &jobThatPanics{}:
				count++
			}
		}
	}()

	return out
}
