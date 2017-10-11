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
*/

package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
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
type LocalOrdered struct {
	started    bool
	numStarted int
	channel    chan amboy.Job
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
func NewLocalOrdered(workers int) *LocalOrdered {
	q := &LocalOrdered{
		channel: make(chan amboy.Job, 100),
	}
	q.tasks.m = make(map[string]amboy.Job)
	q.tasks.ids = make(map[string]int64)
	q.tasks.nodes = make(map[int64]amboy.Job)
	q.tasks.completed = make(map[string]bool)
	q.tasks.graph = simple.NewDirectedGraph()

	r := pool.NewLocalWorkers(workers, q)
	q.runner = r

	return q
}

// Put adds a job to the queue. If the queue has started dispatching
// jobs you cannot add new jobs to the queue. Additionally all jobs
// must have unique names. (i.e. job.ID() values.)
func (q *LocalOrdered) Put(j amboy.Job) error {
	name := j.ID()

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.started {
		return fmt.Errorf("cannot add %s because ordered task dispatching has begun", name)
	}

	if _, ok := q.tasks.m[name]; ok {
		id := q.tasks.ids[name]
		q.tasks.m[name] = j
		q.tasks.nodes[id] = j
		return nil
	}

	node := q.tasks.graph.NewNode()
	id := node.ID()

	q.tasks.m[name] = j
	q.tasks.ids[name] = id
	q.tasks.nodes[id] = j
	q.tasks.graph.AddNode(node)

	return nil
}

// Runner returns the embedded task runner.
func (q *LocalOrdered) Runner() amboy.Runner {
	return q.runner
}

// SetRunner allows users to substitute alternate Runner
// implementations at run time. This method fails if the runner has
// started.
func (q *LocalOrdered) SetRunner(r amboy.Runner) error {
	if q.Started() {
		return errors.New("cannot change runners after starting")
	}

	q.runner = r
	return nil
}

// Started returns true when the Queue has begun dispatching tasks to
// runners.
func (q *LocalOrdered) Started() bool {
	return q.started
}

// Next returns a job from the Queue. This call is non-blocking. If
// there are no pending jobs at the moment, then Next returns an
// error.
func (q *LocalOrdered) Next(ctx context.Context) amboy.Job {
	select {
	case <-ctx.Done():
		return nil
	case job := <-q.channel:
		return job
	}
}

// Results provides an iterator of all "result objects," or completed
// amboy.Job objects. Does not wait for all results to be complete, and is
// closed when all results have been exhausted, even if there are more
// results pending. Other implementations may have different semantics
// for this method.
func (q *LocalOrdered) Results() <-chan amboy.Job {
	q.mutex.RLock()
	output := make(chan amboy.Job, len(q.tasks.completed))
	q.mutex.RUnlock()

	go func() {
		q.mutex.RLock()
		defer q.mutex.RUnlock()
		for _, job := range q.tasks.m {
			if job.Status().Completed {
				output <- job
			}
		}
		close(output)
	}()

	return output
}

// Get takes a name and returns a completed job.
func (q *LocalOrdered) Get(name string) (amboy.Job, bool) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	j, ok := q.tasks.m[name]

	return j, ok
}

// Stats returns a statistics object with data about the total number
// of jobs tracked by the queue.
func (q *LocalOrdered) Stats() amboy.QueueStats {
	s := amboy.QueueStats{}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	s.Completed = len(q.tasks.completed)
	s.Total = len(q.tasks.m)
	s.Pending = s.Total - s.Completed
	s.Running = q.numStarted - s.Completed

	return s
}

func (q *LocalOrdered) buildGraph() error {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	for name, job := range q.tasks.m {
		id, ok := q.tasks.ids[name]
		if !ok {
			return fmt.Errorf("problem building a graph for job %s", name)
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
				return fmt.Errorf("for job %s, the %s dependency is not resolvable [%s]",
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
func (q *LocalOrdered) Start(ctx context.Context) error {
	if q.started {
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
func (q *LocalOrdered) jobDispatch(ctx context.Context, orderedJobs []graph.Node) {
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
			q.Complete(ctx, job)
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
					q.Complete(ctx, job)
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
func (q *LocalOrdered) Complete(ctx context.Context, j amboy.Job) {
	grip.Debugf("marking job (%s) as complete", j.ID())
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.tasks.completed[j.ID()] = true
}
