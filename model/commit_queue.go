package model

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
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
