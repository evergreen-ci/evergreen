package scheduler

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
)

// Function run before sorting all the tasks.  Used to fetch and store
// information needed for prioritizing the tasks.
type sortSetupFunc func(prioritizer *CmpBasedTaskPrioritizer) error

// Get all of the previous completed tasks for the ones to be sorted, and cache
// them appropriately.
func cachePreviousTasks(prioritizer *CmpBasedTaskPrioritizer) (err error) {
	// get the relevant previous completed tasks
	prioritizer.previousTasksCache = make(map[string]task.Task)
	for _, t := range prioritizer.tasks {
		prevTask := &task.Task{}

		// only relevant for repotracker tasks
		if t.Requester == evergreen.RepotrackerVersionRequester {
			prevTask, err = t.PreviousCompletedTask(t.Project, []string{})
			if err != nil {
				return fmt.Errorf("cachePreviousTasks: %v", err)
			}
			if prevTask == nil {
				prevTask = &task.Task{}
			}
		}
		prioritizer.previousTasksCache[t.Id] = *prevTask
	}

	return nil
}

// cacheSimilarFailing fetches all failed tasks with the same display name,
// revision, requester and project but in other buildvariants
func cacheSimilarFailing(prioritizer *CmpBasedTaskPrioritizer) (err error) {
	// find if there are any similar failing tasks
	prioritizer.similarFailingCount = make(map[string]int)
	for _, task := range prioritizer.tasks {
		numSimilarFailing := 0

		// only relevant for repotracker tasks
		if task.Requester == evergreen.RepotrackerVersionRequester {
			numSimilarFailing, err = task.CountSimilarFailingTasks()
			if err != nil {
				return fmt.Errorf("cacheSimilarFailing: %v", err)
			}
		}
		prioritizer.similarFailingCount[task.Id] = numSimilarFailing
	}
	return nil
}
