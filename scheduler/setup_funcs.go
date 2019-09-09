package scheduler

import (
	"runtime"
	"sync"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// Function run before sorting all the tasks.  Used to fetch and store
// information needed for prioritizing the tasks.
type sortSetupFunc func(comparator *CmpBasedTaskComparator) error

// project is a type for holding a subset of the model.Project type.
type project struct {
	TaskGroups []model.TaskGroup `yaml:"task_groups"`
}

// PopulateCaches runs setup functions and is used by the new/tunable
// scheduler to reprocess tasks before running the new planner.
func PopulateCaches(id string, distroID string, tasks []task.Task) ([]task.Task, error) {
	cmp := &CmpBasedTaskComparator{
		tasks:     tasks,
		runtimeID: id,
		setupFuncs: []sortSetupFunc{
			cacheExpectedDurations,
		},
	}
	if err := cmp.setupForSortingTasks(distroID); err != nil {
		return nil, errors.WithStack(err)
	}

	return cmp.tasks, nil
}

func cacheExpectedDurations(comparator *CmpBasedTaskComparator) error {
	work := make(chan task.Task, len(comparator.tasks))
	output := make(chan task.Task, len(comparator.tasks))

	for _, t := range comparator.tasks {
		work <- t
	}
	close(work)

	wg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range work {
				_ = t.FetchExpectedDuration()
				output <- t
			}
		}()
	}
	wg.Wait()

	close(output)
	tasks := make([]task.Task, 0, len(comparator.tasks))

	for t := range output {
		tasks = append(tasks, t)
	}

	comparator.tasks = tasks

	return nil
}
