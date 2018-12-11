/*
Generator

Generators create migration operations and are the first step
in an anser Migration. They are supersets of amboy.Job interfaces.

The current limitation is that the generated jobs must be stored
within the implementation of the generator job, which means they must
either all fit in memory *or* be serializable independently (e.g. fit
in the 16mb document limit if using a MongoDB backed queue.)
*/
package anser

import (
	"context"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Generator is a amboy.Job super set used to store
// implementations that generate other jobs jobs. Internally they
// construct and store member jobs.
//
// Indeed this interface may be useful at some point for any kind of
// job generating operation.
type Generator interface {
	// Jobs produces job objects for the results of the
	// generator.
	Jobs() <-chan amboy.Job

	// Generators are themselves amboy.Jobs.
	amboy.Job
}

// generatorDependency produces a configured dependency.Manager from
// the specified Generator options.
func generatorDependency(env Environment, o model.GeneratorOptions) dependency.Manager {
	// it might be worth considering using some other kind of
	// dependency.Manager implementation.
	dep := env.NewDependencyManager(o.JobID)
	for _, edge := range o.DependsOn {
		dep.AddEdge(edge)
	}
	return dep
}

// addMigrationJobs takes an amboy.Queue, processes the results, and
// adds any jobs produced by the generator to the queue.
func addMigrationJobs(ctx context.Context, q amboy.Queue, dryRun bool, limit int) (int, error) {
	catcher := grip.NewCatcher()
	count := 0
	for job := range q.Results(ctx) {
		generator, ok := job.(Generator)
		if !ok {
			continue
		}
		grip.Infof("adding operations for %s", generator.ID())

		for j := range generator.Jobs() {
			if dryRun {
				grip.Infof("dry-run: would have added %s", j.ID())
				continue
			}

			if limit > 0 && count >= limit {
				return count, catcher.Resolve()
			}
			catcher.Add(q.Put(j))
			count++
		}
	}

	grip.Infof("added %d migration operations", count)
	return count, catcher.Resolve()
}

// generator provides the high level implementation of the Jobs()
// method that's a part of the Generator interface. This
// takes a list of jobs (using a variadic function to do the type
// conversion,) and returns them in a (buffered) channel. with the
// jobs, having had their dependencies set.
func generator(env Environment, groupID string, input <-chan amboy.Job) (<-chan amboy.Job, error) {
	out := make(chan amboy.Job)

	network, err := env.GetDependencyNetwork()

	if err != nil {
		grip.Warning(err)
		close(out)
		return out, errors.WithStack(err)
	}

	go func() {
		defer close(out)
		for migration := range input {
			dep := migration.Dependency()

			for _, group := range network.Resolve(groupID) {
				for _, edge := range network.GetGroup(group) {
					grip.Notice(dep.AddEdge(edge))
				}
			}

			migration.SetDependency(dep)

			out <- migration
		}
	}()

	return out, nil
}
