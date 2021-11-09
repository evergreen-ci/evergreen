/*
Local Ordered Queue

The ordered queue evaluates the dependency information provided by the
tasks and then dispatches tasks to workers to ensure that all
dependencies have run before attempting to run a task.  If there are
cycles in the dependency graph, the queue will not run any tasks. This
implementation is local, in the sense that there is no persistence or
shared state between queue implementations.

By default, LocalOrdered uses the amboy/pool.Workers implementation of
amboy.Runner interface.

The Local Ordered queue requires that *all* jobs be added to the queue
before starting work.
*/

package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// LocalOrdered implements a dependency aware local queue. The queue
// will execute tasks ordered by the topological sort of the
// dependency graph derived from the Edges() output of each job's
// Dependency object. If no task edges are specified, task ordering
// should be roughly equivalent to other non-ordered queues. If there
// are cycles in the dependency graph, the queue will error before
// starting.
type depGraphOrderedLocal struct {
	started    bool
	numStarted int
	id         string
	channel    chan amboy.Job
	dispatcher Dispatcher
	tasks      struct {
		m         map[string]amboy.Job
		ids       map[string]int64
		nodes     map[int64]amboy.Job
		completed map[string]bool
		graph     *simple.DirectedGraph
	}

	// Composed functionality:
	runner amboy.Runner
	mutex  sync.RWMutex
}

// NewLocalOrdered constructs an LocalOrdered object. The "workers"
// argument is passed to a default pool.SimplePool object.
//
// The ordered queue requires that users add all tasks to the queue
// before starting it, and does not accept tasks after starting.
//
// Like other ordered in memory queues, this implementation does not
// support scoped locks.
func NewLocalOrdered(workers int) amboy.Queue {
	q := &depGraphOrderedLocal{
		channel: make(chan amboy.Job, workers*10),
	}
	q.tasks.m = make(map[string]amboy.Job)
	q.tasks.ids = make(map[string]int64)
	q.tasks.nodes = make(map[int64]amboy.Job)
	q.tasks.completed = make(map[string]bool)
	q.tasks.graph = simple.NewDirectedGraph()
	q.id = fmt.Sprintf("queue.local.ordered.graph.%s", uuid.New().String())
	r := pool.NewLocalWorkers(workers, q)
	q.runner = r
	q.dispatcher = NewDispatcher(q)
	return q
}

func (q *depGraphOrderedLocal) ID() string {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.id
}

// Put adds a job to the queue. If the queue has started dispatching
// jobs you cannot add new jobs to the queue. Additionally all jobs
// must have unique names. (i.e. job.ID() values.)
func (q *depGraphOrderedLocal) Put(ctx context.Context, j amboy.Job) error {
	name := j.ID()

	j.UpdateTimeInfo(amboy.JobTimeInfo{
		Created: time.Now(),
	})

	if err := j.TimeInfo().Validate(); err != nil {
		return errors.Wrap(err, "invalid job timeinfo")
	}
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.started {
		return errors.Errorf("cannot add %s because ordered task dispatching has begun", name)
	}

	if _, ok := q.tasks.m[name]; ok {
		return amboy.NewDuplicateJobErrorf("cannot add %s because duplicate job ids are not allowed", name)
	}

	node := q.tasks.graph.NewNode()
	id := node.ID()

	q.tasks.m[name] = j
	q.tasks.ids[name] = id
	q.tasks.nodes[id] = j
	q.tasks.graph.AddNode(node)

	return nil
}

func (q *depGraphOrderedLocal) Save(ctx context.Context, j amboy.Job) error {
	name := j.ID()

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if !q.started {
		return errors.Errorf("cannot save %s because dispatching has not begun", name)
	}

	if _, ok := q.tasks.m[name]; !ok {
		return amboy.NewJobNotFoundErrorf("cannot add %s because job does not exist", name)
	}

	q.tasks.m[name] = j
	return nil
}

// Runner returns the embedded task runner.
func (q *depGraphOrderedLocal) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows users to substitute alternate Runner
// implementations at run time. This method fails if the runner has
// started.
func (q *depGraphOrderedLocal) SetRunner(r amboy.Runner) error {
	if q.Info().Started {
		return errors.New("cannot change runners after starting")
	}

	q.runner = r
	return nil
}

func (q *depGraphOrderedLocal) Info() amboy.QueueInfo {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.info()
}

func (q *depGraphOrderedLocal) info() amboy.QueueInfo {
	return amboy.QueueInfo{
		Started:     q.started,
		LockTimeout: amboy.LockTimeout,
	}
}

// Next returns a job from the Queue. This call is non-blocking. If
// there are no pending jobs at the moment, then Next returns an
// error.
func (q *depGraphOrderedLocal) Next(ctx context.Context) amboy.Job {
	select {
	case <-ctx.Done():
		return nil
	case job := <-q.channel:
		grip.Error(message.WrapError(q.dispatcher.Dispatch(ctx, job),
			message.Fields{
				"job_id":   job.ID(),
				"event":    "improperly dispatched job",
				"impact":   "possible duplicate execution",
				"queue_id": q.ID(),
			}))
		return job
	}
}

// Results provides an iterator of all "result objects," or completed
// amboy.Job objects. Does not wait for all results to be complete, and is
// closed when all results have been exhausted, even if there are more
// results pending. Other implementations may have different semantics
// for this method.
func (q *depGraphOrderedLocal) Results(ctx context.Context) <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func() {
		q.mutex.RLock()
		defer q.mutex.RUnlock()
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

// JobInfo returns a channel that produces information for all jobs in the
// queue. Job information is returned in no particular order.
func (q *depGraphOrderedLocal) JobInfo(ctx context.Context) <-chan amboy.JobInfo {
	infos := make(chan amboy.JobInfo)
	go func() {
		q.mutex.RLock()
		defer q.mutex.RUnlock()
		defer close(infos)
		for _, j := range q.tasks.m {
			select {
			case <-ctx.Done():
				return
			case infos <- amboy.NewJobInfo(j):
			}

		}
	}()
	return infos
}

// Get takes a name and returns a completed job.
func (q *depGraphOrderedLocal) Get(ctx context.Context, name string) (amboy.Job, bool) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	j, ok := q.tasks.m[name]

	return j, ok
}

// Stats returns a statistics object with data about the total number
// of jobs tracked by the queue.
func (q *depGraphOrderedLocal) Stats(ctx context.Context) amboy.QueueStats {
	s := amboy.QueueStats{}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	s.Completed = len(q.tasks.completed)
	s.Total = len(q.tasks.m)
	s.Pending = s.Total - s.Completed
	s.Running = q.numStarted - s.Completed

	return s
}

func (q *depGraphOrderedLocal) buildGraph() error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for name, job := range q.tasks.m {
		id, ok := q.tasks.ids[name]
		if !ok {
			return errors.Errorf("problem building a graph for job %s", name)
		}

		edges := job.Dependency().Edges()

		if len(edges) == 0 {
			// this won't block because this method is
			// only, called in Start() after the runner
			// has started, so these jobs are processed
			// asap.
			q.channel <- job
			continue
		}

		for _, dep := range edges {
			edgeID, ok := q.tasks.ids[dep]
			if !ok {
				return errors.Errorf("for job %s, the %s dependency is not resolvable [%s]",
					name, dep, strings.Join(edges, ", "))
			}
			edge := simple.Edge{
				F: simple.Node(id),
				T: simple.Node(edgeID),
			}
			q.tasks.graph.SetEdge(edge)
		}
	}

	return nil
}

// Start starts the runner worker processes organizes the graph and
// begins dispatching jobs to the workers.
func (q *depGraphOrderedLocal) Start(ctx context.Context) error {
	if q.Info().Started {
		return nil
	}

	err := q.runner.Start(ctx)
	if err != nil {
		return errors.Wrap(err, "problem starting worker pool")
	}

	q.started = true

	err = q.buildGraph()
	if err != nil {
		return errors.Wrap(err, "problem building dependency graph")
	}

	ordered, err := topo.Sort(q.tasks.graph)
	if err != nil {
		return errors.Wrap(err, "error ordering dependencies")
	}

	go q.jobDispatch(ctx, ordered)

	return nil
}

// Job dispatching that takes an ordering of graph.Nodsand waits for
// dependencies to be resolved before adding them to the queue.
func (q *depGraphOrderedLocal) jobDispatch(ctx context.Context, orderedJobs []graph.Node) {
	// we need to make sure that dependencies don't just get
	// dispatched before their dependents but that they're
	// finished. We iterate through the sorted list in reverse
	// order:
	for i := len(orderedJobs) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return
		default:
		}

		graphItem := orderedJobs[i]

		q.mutex.Lock()
		job := q.tasks.nodes[graphItem.ID()]
		q.numStarted++
		q.mutex.Unlock()

		if job.Dependency().State() == dependency.Passed {
			grip.Warning(message.WrapError(q.Complete(ctx, job), message.Fields{
				"message":  "could not mark job complete",
				"job_id":   job.ID(),
				"queue_id": q.ID(),
			}))
			continue
		}
		if job.Dependency().State() == dependency.Ready {
			q.channel <- job
			continue
		}

		deps := job.Dependency().Edges()
		completedDeps := make(map[string]bool)
	resolveDependencyLoop:
		for {
			select {
			case <-ctx.Done():
				return
			default:
				for _, dep := range deps {
					if completedDeps[dep] {
						// if this is true, then we've
						// seen this task before and
						// we're not waiting for it
						continue
					}

					if q.tasks.completed[dep] || q.tasks.m[dep].Status().Completed {
						// we've not seen this task
						// before, but we're not
						// waiting for it. We'll do a
						// less expensive check in the
						// future.
						completedDeps[dep] = true
					}
					// if neither of the above cases are
					// true, then we're still waiting for
					// a job. might make sense to put a
					// timeout here. On the other hand, if
					// there are cycles in the graph, the
					// topo.Sort should fail, and we'd
					// never get here, so assuming client
					// jobs aren't buggy it's safe enough
					// to wait here.
				}
			}
			if len(deps) == len(completedDeps) {
				// all dependencies have passed, we can try to dispatch the job.

				if job.Dependency().State() == dependency.Passed {
					grip.Warning(message.WrapError(q.Complete(ctx, job), message.Fields{
						"message":  "could not mark job complete",
						"job_id":   job.ID(),
						"queue_id": q.ID(),
					}))
				} else if job.Dependency().State() == dependency.Ready {
					q.channel <- job
				}

				// when the job is dispatched, we can
				// move on to the next item in the ordered queue.
				break resolveDependencyLoop
			}
		}
	}
}

// Complete marks a job as complete in the context of this queue instance.
func (q *depGraphOrderedLocal) Complete(ctx context.Context, j amboy.Job) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	q.dispatcher.Complete(ctx, j)

	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.tasks.completed[j.ID()] = true

	return nil
}

func (q *depGraphOrderedLocal) Close(ctx context.Context) {
	if r := q.Runner(); r != nil {
		r.Close(ctx)
	}
}
