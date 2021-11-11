package model

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v34/github"
	"github.com/pkg/errors"
)

func GetModulesFromPR(ctx context.Context, githubToken string, prNum int, modules []commitqueue.Module, projectConfig *Project) ([]*github.PullRequest, []patch.ModulePatch, error) {
	var modulePRs []*github.PullRequest
	var modulePatches []patch.ModulePatch
	for _, mod := range modules {
		module, err := projectConfig.GetModuleByName(mod.Module)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "can't get module for module name '%s'", mod.Module)
		}
		owner, repo, err := thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "module '%s' misconfigured (malformed URL)", mod.Module)
		}

		pr, err := thirdparty.GetPullRequest(ctx, prNum, githubToken, owner, repo)
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
