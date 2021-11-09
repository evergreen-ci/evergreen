/*
Edges

Dependencies have methods to add or access Edges of the task. These
allow Jobs, by way of their dependencies to express relationships
between Jobs. Fundamentally managing Job ordering is a property of a
Queue implementation, but these methods allow tasks to express their
dependencies on other tasks as a hint to Queue
implementations. Separately, queue implementation checks the
environment to ensure that all prerequisites are satisfied.
*/
package dependency

import "github.com/pkg/errors"

// JobEdges provides a common subset of a non-trivial Manager
// implementation. These objects provide two methods of the Manager
// interface, and keep track of the relationships between Jobs in a
// job queue.
type JobEdges struct {
	TaskEdges []string `bson:"edges" json:"edges" yaml:"edges"`
	edgesSet  map[string]bool
}

// NewJobEdges returns an initialized JobEdges object.
func NewJobEdges() JobEdges {
	return JobEdges{
		TaskEdges: []string{},
		edgesSet:  make(map[string]bool),
	}
}

// Edges returns a copy of JobEdges.Edges list of edges for this
// slice. As a result, adding or removing items from this slice does
// not affect other readers, and this object *only* reflects changes to
// the dependencies made after calling this method.
func (e *JobEdges) Edges() []string {
	output := make([]string, len(e.TaskEdges))
	copy(output, e.TaskEdges)

	return output
}

// AddEdge adds an edge to the dependency tracker. If the edge already
// exists, this operation returns an error.
func (e *JobEdges) AddEdge(name string) error {
	if len(e.TaskEdges) != len(e.edgesSet) {
		// this is probably the case when we're re-reading an
		// instance from the DB. we'll just rehydrate things
		// here.

		for _, edge := range e.TaskEdges {
			e.edgesSet[edge] = true
		}
	}

	if _, ok := e.edgesSet[name]; ok {
		return errors.Errorf("edge '%s' already exists", name)
	}

	// TODO we probably need a lock here, but maybe not?
	e.edgesSet[name] = true
	e.TaskEdges = append(e.TaskEdges, name)

	return nil
}
