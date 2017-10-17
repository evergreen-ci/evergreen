package scheduler

import (
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
)

type TaskFinder func() ([]task.Task, error)

func FindRunnableTasks() ([]task.Task, error) {
	return task.FindRunnable()
}

// The old Task finderDBTaskFinder, with the dependency check implemented in Go,
// instead of using $graphLookup
func LegacyFindRunnableTasks() ([]task.Task, error) {
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
