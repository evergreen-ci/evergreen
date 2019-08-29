package queue

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type adaptiveLocalOrdering struct {
	// the ops are: all map:jobs || ready | blocked | passed+unresolved
	operations chan func(context.Context, *adaptiveOrderItems, *fixedStorage)
	capacity   int
	starter    sync.Once
	id         string
	runner     amboy.Runner
}

// NewAdaptiveOrderedLocalQueue provides a queue implementation that
// stores jobs in memory, and dispatches tasks based on the dependency
// information.
//
// Use this implementation rather than LocalOrderedQueue when you need
// to add jobs *after* starting the queue, and when you want to avoid
// the higher potential overhead of the remote-backed queues.
func NewAdaptiveOrderedLocalQueue(workers, capacity int) amboy.Queue {
	q := &adaptiveLocalOrdering{}
	r := pool.NewLocalWorkers(workers, q)
	q.capacity = capacity
	q.runner = r
	q.id = fmt.Sprintf("queue.local.ordered.adaptive.%s", uuid.NewV4().String())
	return q
}

func (q *adaptiveLocalOrdering) ID() string { return q.id }

func (q *adaptiveLocalOrdering) Start(ctx context.Context) error {
	if q.runner == nil {
		return errors.New("cannot start queue without a runner")
	}

	q.starter.Do(func() {
		q.operations = make(chan func(context.Context, *adaptiveOrderItems, *fixedStorage))
		go q.reactor(ctx)
		grip.Error(q.runner.Start(ctx))
		grip.Info("started adaptive ordering job rector")
	})

	return nil
}

func (q *adaptiveLocalOrdering) reactor(ctx context.Context) {
	defer recovery.LogStackTraceAndExit("adaptive ordering amboy queue reactor")

	items := &adaptiveOrderItems{
		jobs: make(map[string]amboy.Job),
	}
	fixed := newFixedStorage(q.capacity)

	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case op := <-q.operations:
			op(ctx, items, fixed)
		case <-timer.C:
			items.refilter(ctx)
			timer.Reset(time.Minute)
		}
	}
}

func (q *adaptiveLocalOrdering) Put(ctx context.Context, j amboy.Job) error {
	if !q.Started() {
		return errors.New("cannot add job to unopened queue")
	}

	out := make(chan error)
	op := func(ctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
		defer close(out)
		j.UpdateTimeInfo(amboy.JobTimeInfo{
			Created: time.Now(),
		})
		if err := j.TimeInfo().Validate(); err != nil {
			out <- err
			return
		}

		out <- items.add(j)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.operations <- op:
		return <-out
	}
}

func (q *adaptiveLocalOrdering) Save(ctx context.Context, j amboy.Job) error {
	if !q.Started() {
		return errors.New("cannot add job to unopened queue")
	}

	name := j.ID()
	out := make(chan error)
	op := func(ctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
		defer close(out)
		if _, ok := items.jobs[name]; !ok {
			out <- errors.New("cannot save job that does not exist")
			return
		}

		items.jobs[name] = j
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.operations <- op:
		return <-out
	}
}

func (q *adaptiveLocalOrdering) Get(ctx context.Context, name string) (amboy.Job, bool) {
	if !q.Started() {
		return nil, false
	}

	ret := make(chan amboy.Job)
	op := func(ctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
		defer close(ret)
		if j, ok := items.jobs[name]; ok {
			ret <- j
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
func (q *adaptiveLocalOrdering) Results(ctx context.Context) <-chan amboy.Job {
	ret := make(chan chan amboy.Job)

	op := func(opctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
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

	select {
	case <-ctx.Done():
		out := make(chan amboy.Job)
		close(out)
		return out
	case q.operations <- op:
		return <-ret
	}
}

func (q *adaptiveLocalOrdering) JobStats(ctx context.Context) <-chan amboy.JobStatusInfo {
	ret := make(chan chan amboy.JobStatusInfo)
	op := func(opctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
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

	select {
	case <-ctx.Done():
		out := make(chan amboy.JobStatusInfo)
		close(out)
		return out
	case q.operations <- op:
		return <-ret
	}
}

func (q *adaptiveLocalOrdering) Stats(ctx context.Context) amboy.QueueStats {
	if !q.Started() {
		return amboy.QueueStats{}
	}

	ret := make(chan amboy.QueueStats)
	op := func(ctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
		defer close(ret)
		stat := amboy.QueueStats{
			Total:     len(items.jobs),
			Pending:   len(items.ready) + len(items.waiting) + len(items.stalled),
			Completed: len(items.completed),
		}

		stat.Running = stat.Total - stat.Pending - stat.Completed - len(items.passed)
		ret <- stat
	}

	select {
	case <-ctx.Done():
		return amboy.QueueStats{}
	case q.operations <- op:
		return <-ret
	}
}

func (q *adaptiveLocalOrdering) Started() bool { return q.operations != nil }
func (q *adaptiveLocalOrdering) Next(ctx context.Context) amboy.Job {
	ret := make(chan amboy.Job)
	op := func(ctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
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

	select {
	case <-ctx.Done():
		return nil
	case q.operations <- op:
		return <-ret
	}
}

func (q *adaptiveLocalOrdering) Complete(ctx context.Context, j amboy.Job) {
	if ctx.Err() != nil {
		return
	}
	wait := make(chan struct{})
	op := func(ctx context.Context, items *adaptiveOrderItems, fixed *fixedStorage) {
		id := j.ID()
		items.completed = append(items.completed, id)
		items.jobs[id] = j
		fixed.Push(id)

		if num := fixed.Oversize(); num > 0 {
			for i := 0; i < num; i++ {
				items.remove(fixed.Pop())
			}
		}

		close(wait)
	}

	select {
	case <-ctx.Done():
	case q.operations <- op:
		<-wait
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
