package model

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/v52/github"
	"github.com/pkg/errors"
)

func GetModulesFromPR(ctx context.Context, modules []commitqueue.Module, projectConfig *Project) ([]*github.PullRequest, []patch.ModulePatch, error) {
	var modulePRs []*github.PullRequest
	var modulePatches []patch.ModulePatch
	for _, mod := range modules {
		module, err := projectConfig.GetModuleByName(mod.Module)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting module for module name '%s'", mod.Module)
		}
		owner, repo, err := module.GetOwnerAndRepo()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting owner and repo for '%s'", mod.Module)
		}

		prNum, err := strconv.Atoi(mod.Issue)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "malformed PR number for module '%s'", mod.Module)
		}
		pr, err := thirdparty.GetMergeablePullRequest(ctx, prNum, owner, repo)
		if err != nil {
			return nil, nil, errors.Wrap(err, "PR not valid for merge")
		}
		modulePRs = append(modulePRs, pr)
		githash := pr.GetMergeCommitSHA()

		modulePatches = append(modulePatches, patch.ModulePatch{
			ModuleName: mod.Module,
			Githash:    githash,
			PatchSet: patch.PatchSet{
				Patch: mod.Issue,
			},
		})
	}

	return modulePRs, modulePatches, nil
}

// CommitQueueRemoveItem dequeues an item from the commit queue and returns the
// removed item. If the item is already being tested in a batch, later items in
// the batch are restarted.
func CommitQueueRemoveItem(ctx context.Context, cq *commitqueue.CommitQueue, item commitqueue.CommitQueueItem, user, reason string) (*commitqueue.CommitQueueItem, error) {
	if item.Version != "" {
		// If the patch has been finalized, it may already be running in a
		// batch, so it has to restart later items that are running in its
		// batch.
		removed, err := DequeueAndRestartForVersion(ctx, cq, cq.ProjectID, item.Version, user, reason)
		if err != nil {
			return nil, errors.Wrap(err, "dequeueing and restarting finalized commit queue item")
		}
		return removed, nil
	}

	// If the patch hasn't been finalized yet, it can simply be removed from
	// the commit queue.
	removed, err := RemoveItemAndPreventMerge(cq, item.Issue, user)
	if err != nil {
		return nil, errors.Wrap(err, "removing unfinalized commit queue item")
	}
	if removed == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Errorf("item '%s' not found in commit queue", item.Issue).Error(),
		}
	}
	return removed, nil
}

func RemoveCommitQueueItemForVersion(projectId, version string, user string) (*commitqueue.CommitQueueItem, error) {
	cq, err := commitqueue.FindOneId(projectId)
	if err != nil {
		return nil, errors.Wrapf(err, "getting commit queue for project '%s'", projectId)
	}
	if cq == nil {
		return nil, errors.Errorf("no commit queue found for project '%s'", projectId)
	}

	issue := ""
	for _, item := range cq.Queue {
		if item.Version == version {
			issue = item.Issue
		}
	}
	if issue == "" {
		return nil, nil
	}

	return RemoveItemAndPreventMerge(cq, issue, user)
}

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
