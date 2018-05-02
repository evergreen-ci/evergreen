package scheduler

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Comparator (-1 if second is more important, 1 if first is, 0 if equal)
// takes in the task comparator because it may need access to additional info
// beyond just what's in the tasks.
type taskPriorityCmp func(task.Task, task.Task, *CmpBasedTaskComparator) (int, error)

// Importance comparison functions for tasks.  Used to prioritize tasks by the
// CmpBasedTaskComparator.

// byPriority compares the explicit Priority field of the Task documents for
// each Task.  The Task whose Priority field is higher will be considered
// more important.
func byPriority(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, error) {
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
func byNumDeps(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, error) {
	if t1.NumDependents > t2.NumDependents {
		return 1, nil
	}
	if t1.NumDependents < t2.NumDependents {
		return -1, nil
	}

	return 0, nil
}

// byAge replaces previous attempts to prioritize tasks by age. The
// previous behavior always preferred newer tasks over older tasks (to
// reduce redundant work on the assumption that newer tasks would be
// better.) "Neweness" was revision-order number when tasks were from
// the same project, or creation time otherwise. This meant that tasks
// could hang out in the middle of the queue forever.
//
// By removing the old creation time/order number sorters, we're
// imposing a new policy with the following goals, based on the
// premise that Evergreen is a multi-tenant system.
//
// - Two (commit) tasks of the same project should prefer the newer task.
// - Two (commit) tasks of different project should prefer the older task.
// - Two patch builds should prefer the older task.
func byAge(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, error) {
	if tasksAreCommitBuilds(t1, t2) && tasksAreFromOneProject(t1, t2) {
		if t1.RevisionOrderNumber > t2.RevisionOrderNumber {
			return 1, nil
		} else if t1.RevisionOrderNumber < t2.RevisionOrderNumber {
			return -1, nil
		} else {
			return 0, nil
		}
	}

	if t1.CreateTime.Before(t2.CreateTime) {
		return 1, nil
	} else if t2.CreateTime.Before(t1.CreateTime) {
		return -1, nil
	} else {
		return 0, nil
	}
}

// byRuntime orders tasks so that the tasks that we expect to take
// longer will start before the tasks that we expect to take less time,
// which we expect will shorten makespan (without shortening total
// runtime,) leading to faster feedback for users.
func byRuntime(t1, t2 task.Task, comp *CmpBasedTaskComparator) (int, error) {
	prevOne, okOne := comp.previousTasksCache[t1.Id]
	prevTwo, okTwo := comp.previousTasksCache[t2.Id]
	if !okOne || !okTwo {
		return 0, nil
	}

	if prevOne.TimeTaken == 0 || prevTwo.TimeTaken == 0 {
		return 0, nil
	}

	if prevOne.TimeTaken == prevTwo.TimeTaken {
		return 0, nil
	}

	if prevOne.TimeTaken > prevTwo.TimeTaken {
		return 1, nil
	}

	return -1, nil
}

// byRecentlyFailing compares the results of the previous executions of each
// Task, and considers one more important if its previous execution resulted in
// failure.
func byRecentlyFailing(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, error) {
	firstPrev, present := comparator.previousTasksCache[t1.Id]
	if !present {
		return 0, errors.Errorf("No cached previous task available for task with"+
			" id %v", t1.Id)
	}
	secondPrev, present := comparator.previousTasksCache[t2.Id]
	if !present {
		return 0, errors.Errorf("No cached previous task available for task with"+
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
func bySimilarFailing(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, error) {
	// this comparator only applies to tasks within the same revision
	if t1.Revision != t2.Revision {
		return 0, nil
	}

	numSimilarFailingOne, ok := comparator.similarFailingCount[t1.Id]
	if !ok {
		return 0, errors.Errorf("No similar failing count entry for task with "+
			"id %v", t1.Id)
	}

	numSimilarFailingTwo, ok := comparator.similarFailingCount[t2.Id]
	if !ok {
		return 0, errors.Errorf("No similar failing count entry for task with "+
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

// utilities

func tasksAreFromOneProject(t1, t2 task.Task) bool { return t1.Project == t2.Project }
func tasksAreCommitBuilds(t1, t2 task.Task) bool {
	if t1.Requester == evergreen.RepotrackerVersionRequester && t2.Requester == evergreen.RepotrackerVersionRequester {
		return true
	}
	return false
}

// byTaskGroupOrder takes two tasks with the same build and non-empty task group
// and considers one more important if it appears earlier in the task group task
// list. This is to ensure that task groups are dispatched in the order that
// they are defined.
func byTaskGroupOrder(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, error) {
	if differentBuildOrTaskGroup(t1, t2) {
		return 0, nil
	}

	// find earlier task
	for _, tg := range comparator.projects[t1.Version].TaskGroups {
		if tg.Name == t1.TaskGroup {
			for _, t := range tg.Tasks {
				if t == t1.DisplayName {
					return 1, nil
				}
				if t == t2.DisplayName {
					return -1, nil
				}
			}
		}
	}
	tasksFromTaskGroup := []string{}
	for _, tg := range comparator.projects[t1.Version].TaskGroups {
		if tg.Name == t1.TaskGroup {
			tasksFromTaskGroup = append(tasksFromTaskGroup, tg.Tasks...)
		}
	}
	grip.Error(message.Fields{
		"message":                 "something went wrong sorting by task group order",
		"ticket":                  "EVG-3305",
		"t1_id":                   t1.Id,
		"t1_version":              t1.Version,
		"t1_group":                t1.TaskGroup,
		"t2_id":                   t2.Id,
		"t2_version":              t2.Version,
		"comparator_projects_len": len(comparator.projects),
		"tasks_from_task_group":   tasksFromTaskGroup,
	})
	return 0, errors.Errorf("did not find tasks %s or %s in task group %s", t1.DisplayName, t2.DisplayName, t1.TaskGroup)
}

// differentBuildOrTaskGroup returns true iff two tasks have different builds
// or task groups, or empty task groups.
func differentBuildOrTaskGroup(t1, t2 task.Task) bool {
	if t1.TaskGroup == "" || t2.TaskGroup == "" ||
		t1.TaskGroup != t2.TaskGroup ||
		t1.BuildId != t2.BuildId {
		return true
	}
	return false
}

// byGenerateTasks schedules tasks that generate tasks ahead of tasks that do not.
func byGenerateTasks(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, error) {
	if t1.GenerateTask == t2.GenerateTask {
		return 0, nil
	}
	if t1.GenerateTask {
		return 1, nil
	}
	return -1, nil
}
