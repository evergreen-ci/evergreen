package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubModuleRepoGitHubChecks makes the repos in publicRepos public and lets
// each user read the repos in readableRepos.
func stubModuleRepoGitHubChecks(t *testing.T, publicRepos []string, readableRepos map[string][]string) {
	origPublic, origRead := isGitHubRepoPublic, gitHubUserHasReadPermission
	t.Cleanup(func() {
		isGitHubRepoPublic, gitHubUserHasReadPermission = origPublic, origRead
	})
	isGitHubRepoPublic = func(_ context.Context, owner, repo string) (bool, error) {
		if owner == "uninstalled" {
			return false, errors.Wrap(githubapp.ErrGitHubAppNotInstalled, "getting installation token")
		}
		for _, r := range publicRepos {
			if r == owner+"/"+repo {
				return true, nil
			}
		}
		return false, nil
	}
	gitHubUserHasReadPermission = func(_ context.Context, owner, repo, username string) (bool, error) {
		for _, r := range readableRepos[username] {
			if r == owner+"/"+repo {
				return true, nil
			}
		}
		return false, nil
	}
}

func TestValidatePatchModuleRepos(t *testing.T) {
	require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection, manifest.Collection, ProjectVarsCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection, manifest.Collection, ProjectVarsCollection))
	})

	projectRef := &ProjectRef{Id: "trust-project", Owner: "myorg", Repo: "myrepo"}
	require.NoError(t, projectRef.Insert(t.Context()))
	const baseRevision = "base-revision"
	baseVersion := Version{
		Id:                  "base-version",
		Identifier:          projectRef.Id,
		Revision:            baseRevision,
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 1,
	}
	baseManifest := manifest.Manifest{
		Id:          baseVersion.Id,
		ProjectName: projectRef.Id,
		Revision:    baseRevision,
		IsBase:      true,
		Modules: map[string]*manifest.Module{
			"base-mod": {Owner: "myorg", Repo: "base-module"},
		},
	}
	recentVersion := Version{
		Id:                  "recent-version",
		Identifier:          projectRef.Id,
		Revision:            "recent-revision",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 2,
	}
	recentManifest := manifest.Manifest{
		Id:          recentVersion.Id,
		ProjectName: projectRef.Id,
		Revision:    recentVersion.Revision,
		IsBase:      true,
		Modules: map[string]*manifest.Module{
			"recent-mod": {Owner: "otherorg", Repo: "recent-module"},
		},
	}
	require.NoError(t, baseVersion.Insert(t.Context()))
	require.NoError(t, recentVersion.Insert(t.Context()))
	require.NoError(t, baseManifest.Insert(t.Context()))
	require.NoError(t, recentManifest.Insert(t.Context()))
	vars := ProjectVars{Id: projectRef.Id, Vars: map[string]string{"module_owner": "myorg"}}
	_, err := vars.Upsert(t.Context())
	require.NoError(t, err)

	stubModuleRepoGitHubChecks(t, []string{"someorg/public-repo"}, map[string][]string{
		"pr-author": {"secretorg/pr-readable"},
	})

	cliPatch := &patch.Patch{Id: mgobson.NewObjectId(), Project: projectRef.Id, Githash: baseRevision}
	prPatch := &patch.Patch{
		Id:              mgobson.NewObjectId(),
		Project:         projectRef.Id,
		Githash:         baseRevision,
		GithubPatchData: thirdparty.GithubPatch{HeadOwner: "fork-owner", Author: "pr-author"},
	}

	for name, tc := range map[string]struct {
		p         *patch.Patch
		module    Module
		untrusted bool
	}{
		"ModuleInBaseManifestIsTrusted": {
			p:      cliPatch,
			module: Module{Name: "base-mod", Owner: "myorg", Repo: "base-module"},
		},
		"ModuleInRecentMainlineManifestIsTrusted": {
			p:      cliPatch,
			module: Module{Name: "new-name", Owner: "OtherOrg", Repo: "recent-module.git"},
		},
		"ProjectRepoIsTrusted": {
			p:      cliPatch,
			module: Module{Name: "self", Repo: "git@github.com:myorg/myrepo.git"},
		},
		"WikiOfTrustedRepoIsTrusted": {
			p:      cliPatch,
			module: Module{Name: "wiki", Owner: "myorg", Repo: "base-module.wiki"},
		},
		"PublicRepoIsTrusted": {
			p:      cliPatch,
			module: Module{Name: "public", Owner: "someorg", Repo: "public-repo"},
		},
		"RepoWithoutGitHubAppInstallationIsTrusted": {
			p:      cliPatch,
			module: Module{Name: "uninstalled", Owner: "uninstalled", Repo: "repo"},
		},
		"UndeclaredRepoInCLIPatchIsUntrusted": {
			p:         cliPatch,
			module:    Module{Name: "secret", Owner: "secretorg", Repo: "secret-repo"},
			untrusted: true,
		},
		"DeclaredModuleNameWithNewRepoIsUntrusted": {
			p:         cliPatch,
			module:    Module{Name: "base-mod", Owner: "secretorg", Repo: "secret-repo"},
			untrusted: true,
		},
		"OwnerIsExpandedBeforeCheck": {
			p:      cliPatch,
			module: Module{Name: "expanded", Owner: "${module_owner}", Repo: "base-module"},
		},
		"RepoPRAuthorCanReadIsTrustedForPRPatch": {
			p:      prPatch,
			module: Module{Name: "pr", Owner: "secretorg", Repo: "pr-readable"},
		},
		"RepoPRAuthorCanReadIsUntrustedForCLIPatch": {
			p:         cliPatch,
			module:    Module{Name: "pr", Owner: "secretorg", Repo: "pr-readable"},
			untrusted: true,
		},
		"RepoPRAuthorCannotReadIsUntrusted": {
			p:         prPatch,
			module:    Module{Name: "secret", Owner: "secretorg", Repo: "secret-repo"},
			untrusted: true,
		},
		"MergeQueuePatchIsNotChecked": {
			p: &patch.Patch{
				Id:              mgobson.NewObjectId(),
				Project:         projectRef.Id,
				GithubMergeData: thirdparty.GithubMergeGroup{HeadSHA: "merge-sha"},
			},
			module: Module{Name: "secret", Owner: "secretorg", Repo: "secret-repo"},
		},
		"ChildPatchIsNotChecked": {
			p: &patch.Patch{
				Id:       mgobson.NewObjectId(),
				Project:  projectRef.Id,
				Triggers: patch.TriggerInfo{ParentPatch: "parent"},
			},
			module: Module{Name: "secret", Owner: "secretorg", Repo: "secret-repo"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidatePatchModuleRepos(t.Context(), tc.p, projectRef, ModuleList{tc.module})
			if tc.untrusted {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.module.Name)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRetrieveFileForModuleRejectsUntrustedRepo(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, manifest.Collection))
	stubModuleRepoGitHubChecks(t, nil, nil)

	projectRef := &ProjectRef{Id: "trust-project", Owner: "myorg", Repo: "myrepo"}
	p := &patch.Patch{Id: mgobson.NewObjectId(), Project: projectRef.Id, Githash: "revision"}
	opts := GetProjectOpts{
		Ref:               projectRef,
		ReadFileFrom:      ReadFromPatch,
		RemotePath:        "include.yml",
		moduleRepoChecker: newPatchModuleRepoChecker(projectRef, p),
	}
	modules := ModuleList{{Name: "secret", Owner: "secretorg", Repo: "secret-repo", Branch: "main"}}

	_, err := retrieveFileForModule(t.Context(), opts, modules, parserInclude{FileName: "include.yml", Module: "secret"}, nil, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretorg/secret-repo")
}

func TestIsRepoAllowedForTask(t *testing.T) {
	settings := testutil.TestConfig()
	require.NoError(t, db.ClearCollections(
		task.Collection, ProjectRefCollection,
		VersionCollection, ParserProjectCollection,
		manifest.Collection, patch.Collection,
	))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(
			task.Collection, ProjectRefCollection,
			VersionCollection, ParserProjectCollection,
			manifest.Collection, patch.Collection,
		))
	})
	stubModuleRepoGitHubChecks(t, nil, nil)

	pRef := ProjectRef{
		Id:    "p1",
		Owner: "myorg",
		Repo:  "myrepo",
	}
	v := Version{
		Id:         "v1",
		Identifier: "p1",
		Revision:   "abc123",
		Requester:  evergreen.RepotrackerVersionRequester,
	}
	pp := ParserProject{
		Id: "v1",
		Modules: []Module{
			{Name: "mod1", Owner: "myorg", Repo: "mod-repo"},
			{Name: "mod2", Owner: "otherorg", Repo: "other-repo"},
		},
	}
	tsk := task.Task{
		Id:        "t1",
		Project:   "p1",
		Version:   "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	require.NoError(t, pRef.Insert(t.Context()))
	require.NoError(t, v.Insert(t.Context()))
	require.NoError(t, pp.Insert(t.Context()))
	require.NoError(t, tsk.Insert(t.Context()))

	// The patch's manifest declares a module the mainline manifest doesn't.
	p := patch.Patch{Id: mgobson.NewObjectId(), Project: "p1", Githash: "abc123"}
	p.Version = p.Id.Hex()
	require.NoError(t, p.Insert(t.Context()))
	mainlineManifest := manifest.Manifest{
		Id:          "v1",
		ProjectName: "p1",
		Revision:    "abc123",
		IsBase:      true,
		Modules: map[string]*manifest.Module{
			"mod1": {Owner: "myorg", Repo: "mod-repo"},
			"mod2": {Owner: "otherorg", Repo: "other-repo"},
		},
	}
	patchManifest := manifest.Manifest{
		Id:          p.Id.Hex(),
		ProjectName: "p1",
		Revision:    "abc123",
		Modules: map[string]*manifest.Module{
			"mod1":   {Owner: "myorg", Repo: "mod-repo"},
			"secret": {Owner: "secretorg", Repo: "secret-repo"},
		},
	}
	require.NoError(t, mainlineManifest.Insert(t.Context()))
	require.NoError(t, patchManifest.Insert(t.Context()))
	patchTask := task.Task{
		Id:        "patch-task",
		Project:   "p1",
		Version:   p.Id.Hex(),
		Requester: evergreen.PatchVersionRequester,
	}

	for name, tc := range map[string]struct {
		task    *task.Task
		owner   string
		repo    string
		allowed bool
	}{
		"ProjectRepoIsAllowed": {
			task:    &tsk,
			owner:   "myorg",
			repo:    "myrepo",
			allowed: true,
		},
		"ProjectRepoIsCaseInsensitive": {
			task:    &tsk,
			owner:   "MyOrg",
			repo:    "MyRepo",
			allowed: true,
		},
		"DeclaredModuleIsAllowed": {
			task:    &tsk,
			owner:   "myorg",
			repo:    "mod-repo",
			allowed: true,
		},
		"DeclaredModuleDifferentOrgInMainlineIsAllowed": {
			task:    &tsk,
			owner:   "otherorg",
			repo:    "other-repo",
			allowed: true,
		},
		"UndeclaredRepoIsRejected": {
			task:    &tsk,
			owner:   "myorg",
			repo:    "secret-repo",
			allowed: false,
		},
		"UndeclaredOwnerIsRejected": {
			task:    &tsk,
			owner:   "attacker",
			repo:    "myrepo",
			allowed: false,
		},
		"PatchModuleInMainlineManifestIsAllowed": {
			task:    &patchTask,
			owner:   "myorg",
			repo:    "mod-repo",
			allowed: true,
		},
		"PatchModuleNotInMainlineIsRejected": {
			task:    &patchTask,
			owner:   "secretorg",
			repo:    "secret-repo",
			allowed: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			allowed, err := IsRepoAllowedForTask(t.Context(), settings, tc.task, tc.owner, tc.repo)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowed)
		})
	}
}
