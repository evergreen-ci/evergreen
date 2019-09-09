package scheduler

import (
	"fmt"
	"runtime"
	"sort"
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

// groupTaskGroups puts tasks that have the same build and task group next to
// each other in the queue. This ensures that, in a stable sort,
// byTaskGroupOrder sorts task group members relative to each other.
func groupTaskGroups(comparator *CmpBasedTaskComparator) error {
	taskMap := make(map[string]task.Task)
	taskKeys := []string{}
	for _, t := range comparator.tasks {
		k := fmt.Sprintf("%s-%s-%s", t.BuildId, t.TaskGroup, t.Id)
		taskMap[k] = t
		taskKeys = append(taskKeys, k)
	}
	// Reverse sort to sort task groups to the top, so that they are more
	// quickly pinned to hosts.
	sort.Sort(sort.Reverse(sort.StringSlice(taskKeys)))
	for i, k := range taskKeys {
		comparator.tasks[i] = taskMap[k]
	}
	return nil
}
