package scheduler

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
)

// Comparator (-1 if second is more important, 1 if first is, 0 if equal)
// takes in the task prioritizer because it may need access to additional info
//  beyond just what's in the tasks
type taskPriorityCmp func(task.Task, task.Task, *CmpBasedTaskPrioritizer) (
	int, error)

// Importance comparison functions for tasks.  Used to prioritize tasks by the
// CmpBasedTaskPrioritizer.

// byPriority compares the explicit Priority field of the Task documents for
// each Task.  The Task whose Priority field is higher will be considered
// more important.
func byPriority(t1, t2 task.Task, prioritizer *CmpBasedTaskPrioritizer) (int,
	error) {
	if t1.Priority > t2.Priority {
		return 1, nil
	}
	if t1.Priority < t2.Priority {
		return -1, nil
	}

	return 0, nil
}

// byNumDeps compares the NumDependents field of the Task documents for
// each Task.  The Task whose NumDependents field is higher will be considered
// more important.
func byNumDeps(t1, t2 task.Task, prioritizer *CmpBasedTaskPrioritizer) (int,
	error) {
	if t1.NumDependents > t2.NumDependents {
		return 1, nil
	}
	if t1.NumDependents < t2.NumDependents {
		return -1, nil
	}

	return 0, nil
}

// byRevisionOrderNumber compares the RevisionOrderNumber fields for the two Tasks,
// and considers the one with the higher RevisionOrderNumber to be more important.
// byRevisionOrderNumber short circuits if the two tasks are not of the same
// project, since RevisionOrderNumber is meaningless across projects.
func byRevisionOrderNumber(t1, t2 task.Task,
	prioritizer *CmpBasedTaskPrioritizer) (int, error) {

	if t1.Project != t2.Project {
		return 0, nil
	}
	if t1.RevisionOrderNumber > t2.RevisionOrderNumber {
		return 1, nil
	}
	if t2.RevisionOrderNumber > t1.RevisionOrderNumber {
		return -1, nil
	}
	return 0, nil
}

// byCreateTime compares the CreateTime fields of the two tasks, and considers
// the task with the later CreateTime to be more important.  byCreateTime
// short-circuits if the two tasks are from the same project, since
// RevisionOrderNumber is a more reliable indicator on the same project.
func byCreateTime(t1, t2 task.Task, prioritizer *CmpBasedTaskPrioritizer) (int,
	error) {

	if t1.Project == t2.Project {
		return 0, nil
	}

	if t1.CreateTime.After(t2.CreateTime) {
		return 1, nil
	}
	if t2.CreateTime.After(t1.CreateTime) {
		return -1, nil
	}
	return 0, nil
}

// byRecentlyFailing compares the results of the previous executions of each
// Task, and considers one more important if its previous execution resulted in
// failure.
func byRecentlyFailing(t1, t2 task.Task,
	prioritizer *CmpBasedTaskPrioritizer) (int, error) {
	firstPrev, present := prioritizer.previousTasksCache[t1.Id]
	if !present {
		return 0, fmt.Errorf("No cached previous task available for task with"+
			" id %v", t1.Id)
	}
	secondPrev, present := prioritizer.previousTasksCache[t2.Id]
	if !present {
		return 0, fmt.Errorf("No cached previous task available for task with"+
			" id %v", t2.Id)
	}

	if firstPrev.Status == evergreen.TaskFailed &&
		secondPrev.Status != evergreen.TaskFailed {
		return 1, nil
	}
	if secondPrev.Status == evergreen.TaskFailed &&
		firstPrev.Status != evergreen.TaskFailed {
		return -1, nil
	}

	return 0, nil
}

// bySimilarFailing takes two tasks with the same revision, and considers one
// more important if it has a greater number of failed tasks with the same
// revision, project, display name and requester (but in one or more
// buildvariants) that failed.
func bySimilarFailing(t1, t2 task.Task,
	prioritizer *CmpBasedTaskPrioritizer) (int, error) {
	// this comparator only applies to tasks within the same revision
	if t1.Revision != t2.Revision {
		return 0, nil
	}

	numSimilarFailingOne, ok := prioritizer.similarFailingCount[t1.Id]
	if !ok {
		return 0, fmt.Errorf("No similar failing count entry for task with "+
			"id %v", t1.Id)
	}

	numSimilarFailingTwo, ok := prioritizer.similarFailingCount[t2.Id]
	if !ok {
		return 0, fmt.Errorf("No similar failing count entry for task with "+
			"id %v", t2.Id)
	}

	if numSimilarFailingOne > numSimilarFailingTwo {
		return 1, nil
	}

	if numSimilarFailingOne < numSimilarFailingTwo {
		return -1, nil
	}
	return 0, nil
}
