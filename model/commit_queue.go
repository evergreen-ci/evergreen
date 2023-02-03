package model

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
)

func GetModulesFromPR(ctx context.Context, githubToken string, modules []commitqueue.Module, projectConfig *Project) ([]*github.PullRequest, []patch.ModulePatch, error) {
	var modulePRs []*github.PullRequest
	var modulePatches []patch.ModulePatch
	for _, mod := range modules {
		module, err := projectConfig.GetModuleByName(mod.Module)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "getting module for module name '%s'", mod.Module)
		}
		owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "malformed URL for module '%s'", mod.Module)
		}

		prNum, err := strconv.Atoi(mod.Issue)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "malformed PR number for module '%s'", mod.Module)
		}
		pr, err := thirdparty.GetMergeablePullRequest(ctx, prNum, githubToken, owner, repo)
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

func preventMergeForItem(item commitqueue.CommitQueueItem, user string) error {
	// Disable the merge task
	mergeTask, err := task.FindMergeTaskForVersion(item.Version)
	if err != nil {
		return errors.Wrapf(err, "finding merge task for '%s'", item.Issue)
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
