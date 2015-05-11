package scheduler

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
)

// TaskFinder finds all tasks that are ready to be run.
type TaskFinder interface {
	// Returns a slice of tasks that are ready to be run, and an error if
	// appropriate.
	FindRunnableTasks() ([]model.Task, error)
}

// DBTaskFinder fetches tasks from the database. Implements TaskFinder.
type DBTaskFinder struct{}

// FindRunnableTasks finds all tasks that are ready to be run.
// This works by fetching all undispatched tasks from the database,
// and filtering out any whose dependencies are not met.
func (self *DBTaskFinder) FindRunnableTasks() ([]model.Task, error) {

	// find all of the undispatched tasks
	undispatchedTasks, err := model.FindUndispatchedTasks()
	if err != nil {
		return nil, err
	}

	// filter out any tasks whose dependencies are not met
	runnableTasks := make([]model.Task, 0, len(undispatchedTasks))
	dependencyCaches := make(map[string]model.Task)
	for _, task := range undispatchedTasks {
		depsMet, err := task.DependenciesMet(dependencyCaches)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error checking dependencies for"+
				" task %v: %v", task.Id, err)
			continue
		}
		if depsMet {
			runnableTasks = append(runnableTasks, task)
		}
	}

	return runnableTasks, nil
}
