package model

import (
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// RemoveItemAndPreventMerge removes an item from the commit queue and disables the merge task, if applicable.
func RemoveItemAndPreventMerge(cq *commitqueue.CommitQueue, issue string, user string) (*commitqueue.CommitQueueItem, error) {
	removed, err := cq.Remove(issue)
	if err != nil {
		return removed, errors.Wrapf(err, "removing item '%s' from commit queue for project '%s'", issue, cq.ProjectID)
	}

	if removed == nil {
		return nil, nil
	}
	if removed.Version != "" {
		err = preventMergeForItem(*removed, user)
	}

	return removed, errors.Wrapf(err, "preventing merge for item '%s' in commit queue for project '%s'", issue, cq.ProjectID)
}

// preventMergeForItem disables the merge task for a commit queue item to
// prevent it from running.
func preventMergeForItem(item commitqueue.CommitQueueItem, user string) error {
	// Disable the merge task
	mergeTask, err := task.FindMergeTaskForVersion(item.Version)
	if err != nil {
		return errors.Wrapf(err, "finding merge task for item '%s'", item.Issue)
	}
	if mergeTask == nil {
		return errors.New("merge task doesn't exist")
	}
	event.LogMergeTaskUnscheduled(mergeTask.Id, mergeTask.Execution, user)
	if err = DisableTasks(user, *mergeTask); err != nil {
		return errors.Wrap(err, "disabling merge task")
	}

	return nil
}
