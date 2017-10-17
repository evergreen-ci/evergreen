package scheduler

import (
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TaskFinder finds all tasks that are ready to be run.
type TaskFinder interface {
	// Returns a slice of tasks that are ready to be run, and an error if
	// appropriate.
	FindRunnableTasks() ([]task.Task, error)
}

// The old DBTaskFinder, with the dependency check implemented in Go, instead
// of using $graphLookup
type LegacyDBTaskFinder struct{}

func (f *LegacyDBTaskFinder) FindRunnableTasks() ([]task.Task, error) {
	// find all of the undispatched tasks
	undispatchedTasks, err := task.Find(task.IsUndispatched)
	if err != nil {
		return nil, err
	}

	// filter out any tasks whose dependencies are not met
	runnableTasks := make([]task.Task, 0, len(undispatchedTasks))
	dependencyCaches := make(map[string]task.Task)
	for _, task := range undispatchedTasks {
		depsMet, err := task.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Errorf("Error checking dependencies for task %s: %+v", task.Id, err)
			continue
		}
		if depsMet {
			runnableTasks = append(runnableTasks, task)
		}
	}

	return runnableTasks, nil
}

// DBTaskFinder fetches tasks from the database using $graphLookup. Implements TaskFinder.
type DBTaskFinder struct{}

// FindRunnableTasks finds all tasks that are ready to be run.
// This works by fetching all undispatched tasks from the database,
// and filtering out any whose dependencies are not met.
func (f *DBTaskFinder) FindRunnableTasks() ([]task.Task, error) {
	// TODO this func is now redundant
	runnableTasks, err := task.FindRunnable()
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch runnable tasks")
	}

	return runnableTasks, nil
}
