package scheduler

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
)

type taskComparer interface {
	compare(task.Task, task.Task, *CmpBasedTaskComparator) (int, string, error)
	name() string
}

// Importance comparison functions for tasks.  Used to prioritize tasks by the
// CmpBasedTaskComparator.

// byPriority compares the explicit Priority field of the Task documents for
// each Task.  The Task whose Priority field is higher will be considered
// more important.
type byPriority struct{}

func (c *byPriority) name() string { return "task priority" }
func (c *byPriority) compare(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, string, error) {
	reason := "higher is first"
	if t1.Priority > t2.Priority {
		return 1, reason, nil
	}
	if t1.Priority < t2.Priority {
		return -1, reason, nil
	}

	return 0, "", nil
}

// byNumDeps compares the NumDependents field of the Task documents for
// each Task.  The Task whose NumDependents field is higher will be considered
// more important.
type byNumDeps struct{}

func (c *byNumDeps) name() string { return "number of dependencies" }
func (c *byNumDeps) compare(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, string, error) {
	reason := "more dependencies is first"
	if t1.NumDependents > t2.NumDependents {
		return 1, reason, nil
	}
	if t1.NumDependents < t2.NumDependents {
		return -1, reason, nil
	}

	return 0, "", nil
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
type byAge struct{}

func (c *byAge) name() string { return "task age" }
func (c *byAge) compare(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, string, error) {
	reason := ""
	if tasksAreCommitBuilds(t1, t2) && tasksAreFromOneProject(t1, t2) {
		reason = "earlier commit from the same project is first"
		if t1.RevisionOrderNumber > t2.RevisionOrderNumber {
			return 1, reason, nil
		} else if t1.RevisionOrderNumber < t2.RevisionOrderNumber {
			return -1, reason, nil
		} else {
			return 0, "", nil
		}
	}

	reason = "older is first"
	if t1.IngestTime.Before(t2.IngestTime) {
		return 1, reason, nil
	} else if t2.IngestTime.Before(t1.IngestTime) {
		return -1, reason, nil
	} else {
		return 0, "", nil
	}
}

// byRuntime orders tasks so that the tasks that we expect to take
// longer will start before the tasks that we expect to take less time,
// which we expect will shorten makespan (without shortening total
// runtime,) leading to faster feedback for users.
type byRuntime struct{}

func (c *byRuntime) name() string { return "expected runtime" }
func (c *byRuntime) compare(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, string, error) {
	oneExpected := t1.FetchExpectedDuration().Average
	twoExpected := t2.FetchExpectedDuration().Average

	reason := fmt.Sprintf("%s is %s; %s is %s", t1.Id, oneExpected.String(), t2.Id, twoExpected.String())
	if oneExpected == 0 || twoExpected == 0 {
		return 0, "", nil
	}

	if oneExpected == twoExpected {
		return 0, "", nil
	}

	if oneExpected > twoExpected {
		return 1, reason, nil
	}

	return -1, reason, nil
}

// byTaskGroupOrder takes two tasks with the same build and non-empty task group
// and considers one more important if it appears earlier in the task group task
// list. This is to ensure that task groups are dispatched in the order that
// they are defined.
type byTaskGroupOrder struct{}

func (c *byTaskGroupOrder) name() string { return "order within task group" }
func (c *byTaskGroupOrder) compare(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, string, error) {
	// Try other comparators if both tasks are not in task groups
	if t1.TaskGroup == "" && t2.TaskGroup == "" {
		return 0, "", nil
	}

	// If one task is in a task group, sort that one higher, which keeps the pre-byTaskGroupOrder order.
	reason := "the task in a task group is first"
	if t2.TaskGroup == "" && t1.TaskGroup != "" {
		return 1, reason, nil
	}
	if t1.TaskGroup == "" && t2.TaskGroup != "" {
		return -1, reason, nil
	}

	// If tasks are in the same task group and build, apply the task group comparator.
	reason = "earlier in the same task group"
	if t1.TaskGroup == t2.TaskGroup && t1.BuildId == t2.BuildId {
		if t1.TaskGroupOrder > t2.TaskGroupOrder {
			return -1, reason, nil
		}
		if t2.TaskGroupOrder > t1.TaskGroupOrder {
			return 1, reason, nil
		}
	}

	// Otherwise, both tasks are in task groups but in different task groups or builds. Since
	// returning 0 would cause other comparators to run, which could change the task group
	// order, sort them using the same rules as the pre-sort step.
	reason = "different groups, sorting lexically"
	if fmt.Sprintf("%s-%s", t1.BuildId, t1.TaskGroup) < fmt.Sprintf("%s-%s", t2.BuildId, t2.TaskGroup) {
		return 1, reason, nil
	}
	return -1, reason, nil
}

// byGenerateTasks schedules tasks that generate tasks ahead of tasks that do not.
type byGenerateTasks struct{}

func (c *byGenerateTasks) name() string { return "task generator" }
func (c *byGenerateTasks) compare(t1, t2 task.Task, _ *CmpBasedTaskComparator) (int, string, error) {
	if t1.GenerateTask == t2.GenerateTask {
		return 0, "", nil
	}

	reason := "higher task is a generator"
	if t1.GenerateTask {
		return 1, reason, nil
	}
	return -1, reason, nil
}

// byCommitQueue schedules commit queue merges first
type byCommitQueue struct{}

func (c *byCommitQueue) name() string { return "commit queue merge" }
func (c *byCommitQueue) compare(t1, t2 task.Task, comparator *CmpBasedTaskComparator) (int, string, error) {
	reason := "task that is part of the CQ is higher"
	if comparator.versions[t1.Version].Requester == evergreen.MergeTestRequester &&
		comparator.versions[t2.Version].Requester != evergreen.MergeTestRequester {
		return 1, reason, nil
	}
	if comparator.versions[t1.Version].Requester != evergreen.MergeTestRequester &&
		comparator.versions[t2.Version].Requester == evergreen.MergeTestRequester {
		return -1, reason, nil
	}

	return 0, "", nil
}

// utilities

func tasksAreFromOneProject(t1, t2 task.Task) bool { return t1.Project == t2.Project }
func tasksAreCommitBuilds(t1, t2 task.Task) bool {
	if utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t1.Requester) &&
		utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, t1.Requester) {
		return true
	}
	return false
}
