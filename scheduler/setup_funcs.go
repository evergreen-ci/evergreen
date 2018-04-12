package scheduler

import (
	"fmt"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// Function run before sorting all the tasks.  Used to fetch and store
// information needed for prioritizing the tasks.
type sortSetupFunc func(comparator *CmpBasedTaskComparator) error

// Get all of the previous completed tasks for the ones to be sorted, and cache
// them appropriately.
func cachePreviousTasks(comparator *CmpBasedTaskComparator) (err error) {
	// get the relevant previous completed tasks
	comparator.previousTasksCache = make(map[string]task.Task)
	for _, t := range comparator.tasks {
		prevTask := &task.Task{}

		// only relevant for repotracker tasks
		if t.Requester == evergreen.RepotrackerVersionRequester {
			prevTask, err = t.PreviousCompletedTask(t.Project, []string{})
			if err != nil {
				return errors.Wrap(err, "cachePreviousTasks")
			}
			if prevTask == nil {
				prevTask = &task.Task{}
			}
		}
		comparator.previousTasksCache[t.Id] = *prevTask
	}

	return nil
}

// cacheSimilarFailing fetches all failed tasks with the same display name,
// revision, requester and project but in other buildvariants
func cacheSimilarFailing(comparator *CmpBasedTaskComparator) (err error) {
	// find if there are any similar failing tasks
	comparator.similarFailingCount = make(map[string]int)
	for _, task := range comparator.tasks {
		numSimilarFailing := 0

		// only relevant for repotracker tasks
		if task.Requester == evergreen.RepotrackerVersionRequester {
			numSimilarFailing, err = task.CountSimilarFailingTasks()
			if err != nil {
				return errors.Wrap(err, "cacheSimilarFailing")
			}
		}
		comparator.similarFailingCount[task.Id] = numSimilarFailing
	}
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
	sort.Strings(taskKeys)
	for i, k := range taskKeys {
		comparator.tasks[i] = taskMap[k]
	}
	return nil
}
