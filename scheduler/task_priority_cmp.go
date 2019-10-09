package scheduler

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
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

	if t1.IngestTime.Before(t2.IngestTime) {
		return 1, nil
	} else if t2.IngestTime.Before(t1.IngestTime) {
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
	oneExpected := t1.FetchExpectedDuration()
	twoExpected := t2.FetchExpectedDuration()

	if oneExpected == 0 || twoExpected == 0 {
		return 0, nil
	}

	if oneExpected == twoExpected {
		return 0, nil
	}

	if oneExpected > twoExpected {
		return 1, nil
	}

	return -1, nil
}

// utilities

func tasksAreFromOneProject(t1, t2 task.Task) bool { return t1.Project == t2.Project }
func tasksAreCommitBuilds(t1, t2 task.Task) bool {
	if util.StringSliceContains(evergreen.SystemVersionRequesterTypes, t1.Requester) &&
		util.StringSliceContains(evergreen.SystemVersionRequesterTypes, t1.Requester) {
		return true
	}
	return false
}

// byTaskGroupOrder takes two tasks with the same build and non-empty task group
// and considers one more important if it appears earlier in the task group task
// list. This is to ensure that task groups are dispatched in the order that
// they are defined.
func byTaskGroupOrder(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, error) {
	if t1.Version != t2.Version {
		return 0, nil
	}

	if t1.TaskGroup == "" && t2.TaskGroup == "" {
		return 0, nil
	}

	if t2.TaskGroup == "" && t1.TaskGroup != "" {
		return 0, nil
	}

	if t1.TaskGroup == "" && t2.TaskGroup != "" {
		return -1, nil
	}

	if t1.TaskGroup != t2.TaskGroup {
		return 1, nil
	}

	if t1.BuildId != t2.BuildId {
		return 1, nil
	}

	if t1.TaskGroupOrder > t2.TaskGroupOrder {
		return -1, nil
	}

	if t2.TaskGroupOrder > t1.TaskGroupOrder {
		return 1, nil
	}

	return 0, nil
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

func byCommitQueue(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, error) {
	if comparator.versions[t1.Version].Requester == evergreen.MergeTestRequester &&
		comparator.versions[t2.Version].Requester != evergreen.MergeTestRequester {
		return 1, nil
	}
	if comparator.versions[t1.Version].Requester != evergreen.MergeTestRequester &&
		comparator.versions[t2.Version].Requester == evergreen.MergeTestRequester {
		return -1, nil
	}

	return 0, nil
}
