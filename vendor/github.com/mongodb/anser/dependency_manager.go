/*
Dependency Manager

The anser package provides a custom amboy/dependency.Manager object,
which allows migrations to express dependencies to other
migrations. The State() method ensures that all migration IDs
specified as edges are satisfied before reporting as "ready" for work.
*/
package anser

import (
	"context"

	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
)

func init() {
	registry.AddDependencyType("anser-migration", func() dependency.Manager {
		return makeMigrationDependencyManager()
	})
}

type migrationDependency struct {
	MigrationID string              `bson:"migration" json:"migration" yaml:"migration"`
	T           dependency.TypeInfo `bson:"type" json:"type" yaml:"type"`

	MigrationHelper `bson:"-" json:"-" yaml:"-"`
	*dependency.JobEdges
}

func makeMigrationDependencyManager() *migrationDependency {
	edges := dependency.NewJobEdges()
	return &migrationDependency{
		JobEdges: &edges,
		T: dependency.TypeInfo{
			Name:    "anser-migration",
			Version: 0,
		},
	}
}

func (d *migrationDependency) Type() dependency.TypeInfo { return d.T }

func (d *migrationDependency) State() dependency.State {
	edges := d.Edges()
	if len(edges) == 0 {
		return dependency.Ready
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// query the "done" dependencies, and make sure that all the
	// edges listed in the edges document are satisfied.
	return processEdges(ctx, len(edges), d.GetMigrationEvents(ctx, getDependencyStateQuery(edges)))
}

func processEdges(ctx context.Context, numEdges int, iter MigrationMetadataIterator) dependency.State {
	count := 0

	for iter.Next(ctx) {
		meta := iter.Item()
		// if any of the edges are *not* satisfied, then the
		// dependency is by definition blocked
		if !meta.Satisfied() {
			return dependency.Blocked
		}
		count++
	}
	if err := iter.Err(); err != nil {
		grip.Warning(err)
		return dependency.Blocked
	}

	// if we encountered an error in the query, then we should log
	// the error but not let the current task get dispatched.
	if err := iter.Close(); err != nil {
		grip.Warning(err)
		return dependency.Blocked
	}

	// if there are more edges defined than observed in the query,
	// then some tasks haven't reported in or been registered, and
	// we're blocked or stuck
	if count < numEdges {
		return dependency.Blocked
	}

	// otherwise, the task is ready for work:
	return dependency.Ready
}

func getDependencyStateQuery(ids []string) map[string]interface{} {
	return map[string]interface{}{"_id": map[string]interface{}{"$in": ids}}
}
