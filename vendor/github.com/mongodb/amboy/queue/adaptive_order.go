package queue

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type adaptiveLocalOrdering struct {
	// the ops are: all map:jobs || ready | blocked | passed+unresolved
	operations chan func(context.Context, *adaptiveOrderItems)
	starter    sync.Once
	runner     amboy.Runner
}

// NewAdaptiveOrderedLocalQueue provides a queue implementation that
// stores jobs in memory, and dispatches tasks based on the dependency
// information.
//
// Use this implementation rather than LocalOrderedQueue when you need
// to add jobs *after* starting the queue, and when you want to avoid
// the higher potential overhead of the remote-backed queues.
func NewAdaptiveOrderedLocalQueue(workers int) amboy.Queue {
	q := &adaptiveLocalOrdering{}
	r := pool.NewLocalWorkers(workers, q)
	q.runner = r
	return q
}

func (q *adaptiveLocalOrdering) Start(ctx context.Context) error {
	if q.runner == nil {
		return errors.New("cannot start queue without a runner")
	}

	q.starter.Do(func() {
		q.operations = make(chan func(context.Context, *adaptiveOrderItems))
		go q.reactor(ctx)
		grip.CatchError(q.runner.Start(ctx))
		grip.Info("started adaptive ordering job rector")
	})

	return nil
}

func (q *adaptiveLocalOrdering) reactor(ctx context.Context) {
	items := &adaptiveOrderItems{
		jobs: make(map[string]amboy.Job),
	}

	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case op := <-q.operations:
			op(ctx, items)
			timer.Reset(time.Minute)
		case <-timer.C:
			items.refilter(ctx)
			timer.Reset(time.Minute)
		}
	}
}

func (q *adaptiveLocalOrdering) Put(j amboy.Job) error {
	if !q.Started() {
		return errors.New("cannot add job to unopened queue")
	}

	out := make(chan error)
	q.operations <- func(ctx context.Context, items *adaptiveOrderItems) {
		out <- items.add(j)
		close(out)
	}

	return <-out
}

func (q *adaptiveLocalOrdering) Get(name string) (amboy.Job, bool) {
	if !q.Started() {
		return nil, false
	}

	ret := make(chan amboy.Job)

	q.operations <- func(ctx context.Context, items *adaptiveOrderItems) {
		defer close(ret)
		if j, ok := items.jobs[name]; ok {
			ret <- j
		}
	}

	job, ok := <-ret

	return job, ok

}
func (q *adaptiveLocalOrdering) Results(ctx context.Context) <-chan amboy.Job {
	ret := make(chan chan amboy.Job)

	q.operations <- func(opctx context.Context, items *adaptiveOrderItems) {
		out := make(chan amboy.Job, len(items.jobs))
		defer close(ret)
		defer close(out)

		for _, j := range items.jobs {
			if ctx.Err() != nil || opctx.Err() != nil {
				return
			}
			out <- j
		}
		ret <- out
	}

	return <-ret
}

func (q *adaptiveLocalOrdering) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	ret := make(chan chan amboy.JobStatusInfo)

	q.operations <- func(opctx context.Context, items *adaptiveOrderItems) {
		out := make(chan amboy.JobStatusInfo, len(items.jobs))
		defer close(out)
		defer close(ret)

		for _, j := range items.jobs {
			if ctx.Err() != nil || opctx.Err() != nil {
				return
			}

			stat := j.Status()
			stat.ID = j.ID()
			out <- stat
		}
		ret <- out
	}

	return <-ret
}

func (q *adaptiveLocalOrdering) Stats() amboy.QueueStats {
	if !q.Started() {
		return amboy.QueueStats{}
	}

	ret := make(chan amboy.QueueStats)
	q.operations <- func(ctx context.Context, items *adaptiveOrderItems) {
		defer close(ret)
		stat := amboy.QueueStats{
			Total:     len(items.jobs),
			Pending:   len(items.ready) + len(items.waiting) + len(items.stalled),
			Completed: len(items.completed),
		}

		stat.Running = stat.Total - stat.Pending - stat.Completed - len(items.passed)
		ret <- stat
	}

	return <-ret
}

func (q *adaptiveLocalOrdering) Started() bool { return q.operations != nil }
func (q *adaptiveLocalOrdering) Next(ctx context.Context) amboy.Job {
	ret := make(chan amboy.Job)

	q.operations <- func(ctx context.Context, items *adaptiveOrderItems) {
		defer close(ret)

		timer := time.NewTimer(0)
		defer timer.Stop()

		var (
			misses int64
			id     string
		)

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if misses > 10 {
					return
				}

				if len(items.ready) > 0 {
					id, items.ready = items.ready[0], items.ready[1:]
					ret <- items.jobs[id]
					return
				}

				items.refilter(ctx)

				if len(items.ready) > 0 {
					id, items.ready = items.ready[0], items.ready[1:]
					ret <- items.jobs[id]
					return
				}

				misses++
				timer.Reset(time.Duration(misses * rand.Int63n(int64(time.Millisecond))))
			}
		}
	}
	return <-ret
}

func (q *adaptiveLocalOrdering) Complete(ctx context.Context, j amboy.Job) {
	q.operations <- func(ctx context.Context, items *adaptiveOrderItems) {
		items.completed = append(items.completed, j.ID())
	}
}

func (q *adaptiveLocalOrdering) Runner() amboy.Runner { return q.runner }
func (q *adaptiveLocalOrdering) SetRunner(r amboy.Runner) error {
	if q.runner != nil && q.runner.Started() {
		return errors.New("cannot set a runner, current runner is running")
	}

	q.runner = r
	return r.SetQueue(q)
}
