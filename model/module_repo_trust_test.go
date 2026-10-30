package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubModuleRepoGitHubChecks makes the repos in publicRepos public and lets
// each user read the repos in readableRepos. It returns a counter of GitHub
// calls.
func stubModuleRepoGitHubChecks(t *testing.T, publicRepos []string, readableRepos map[string][]string) *int {
	origPublic, origRead := isGitHubRepoPublic, gitHubUserHasReadPermission
	t.Cleanup(func() {
		isGitHubRepoPublic, gitHubUserHasReadPermission = origPublic, origRead
	})
	var calls int
	isGitHubRepoPublic = func(_ context.Context, owner, repo string) (bool, error) {
		calls++
		for _, r := range publicRepos {
			if r == owner+"/"+repo {
				return true, nil
			}
		}
		return false, nil
	}
	gitHubUserHasReadPermission = func(_ context.Context, owner, repo, username string) (bool, error) {
		calls++
		for _, r := range readableRepos[username] {
			if r == owner+"/"+repo {
				return true, nil
			}
		}
		return false, nil
	}
	return &calls
}

func setModuleRepoChecksDisabled(t *testing.T, disabled bool) {
	flags, err := evergreen.GetServiceFlags(t.Context())
	require.NoError(t, err)
	flags.ModuleRepoChecksDisabled = disabled
	require.NoError(t, flags.Set(t.Context()))
	t.Cleanup(func() {
		// The test's context is canceled before cleanup runs.
		flags.ModuleRepoChecksDisabled = false
		assert.NoError(t, flags.Set(context.Background()))
	})
}

func TestValidatePatchModuleRepos(t *testing.T) {
	collections := []string{ProjectRefCollection, VersionCollection, manifest.Collection, ProjectVarsCollection}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})

	projectRef := &ProjectRef{Id: "trust-project", Owner: "myorg", Repo: "myrepo"}
	require.NoError(t, projectRef.Insert(t.Context()))
	const baseRevision = "base-revision"
	versions := []Version{
		{Id: "base-version", Revision: baseRevision, Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 1},
		{Id: "recent-version", Revision: "recent-revision", Requester: evergreen.RepotrackerVersionRequester, RevisionOrderNumber: 2},
		{Id: "tag-version", Revision: "unmerged-revision", Requester: evergreen.GitTagRequester, RevisionOrderNumber: 3},
	}
	manifests := []manifest.Manifest{
		{Id: "base-version", Revision: baseRevision, IsBase: true, Modules: map[string]*manifest.Module{
			"base-mod": {Owner: "myorg", Repo: "base-module"},
		}},
		{Id: "recent-version", Revision: "recent-revision", IsBase: true, Modules: map[string]*manifest.Module{
			"recent-mod": {Owner: "otherorg", Repo: "recent-module"},
		}},
		{Id: "tag-version", Revision: "unmerged-revision", Modules: map[string]*manifest.Module{
			"tag-mod": {Owner: "secretorg", Repo: "tagged-repo"},
		}},
	}
	for _, v := range versions {
		v.Identifier = projectRef.Id
		require.NoError(t, v.Insert(t.Context()))
	}
	for _, m := range manifests {
		m.ProjectName = projectRef.Id
		require.NoError(t, m.Insert(t.Context()))
	}
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
	secretModule := Module{Name: "secret", Owner: "secretorg", Repo: "secret-repo"}

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
		"ModuleOnlyInGitTagVersionIsUntrusted": {
			p:         cliPatch,
			module:    Module{Name: "tag-mod", Owner: "secretorg", Repo: "tagged-repo"},
			untrusted: true,
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
		"UndeclaredRepoInCLIPatchIsUntrusted": {
			p:         cliPatch,
			module:    secretModule,
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
			module:    secretModule,
			untrusted: true,
		},
		"CLIPatchWithMergeQueueAliasIsChecked": {
			p:         &patch.Patch{Id: mgobson.NewObjectId(), Project: projectRef.Id, Githash: baseRevision, Alias: evergreen.CommitQueueAlias},
			module:    secretModule,
			untrusted: true,
		},
		"MergeQueuePatchIsNotChecked": {
			p: &patch.Patch{
				Id:              mgobson.NewObjectId(),
				Project:         projectRef.Id,
				Alias:           evergreen.CommitQueueAlias,
				GithubMergeData: thirdparty.GithubMergeGroup{HeadSHA: "merge-sha"},
			},
			module: secretModule,
		},
		"ChildPatchIsNotChecked": {
			p: &patch.Patch{
				Id:       mgobson.NewObjectId(),
				Project:  projectRef.Id,
				Triggers: patch.TriggerInfo{ParentPatch: "parent"},
			},
			module: secretModule,
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

	t.Run("UntrustedRepoIsAllowedWhenChecksAreDisabled", func(t *testing.T) {
		setModuleRepoChecksDisabled(t, true)
		assert.NoError(t, ValidatePatchModuleRepos(t.Context(), cliPatch, projectRef, ModuleList{secretModule}))
	})
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

func TestRecordModuleRepoTrust(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, manifest.Collection))
	stubModuleRepoGitHubChecks(t, []string{"someorg/public-repo"}, map[string][]string{
		"pr-author": {"secretorg/pr-readable"},
	})

	projectRef := &ProjectRef{Id: "trust-project", Owner: "myorg", Repo: "myrepo"}
	p := &patch.Patch{
		Id:              mgobson.NewObjectId(),
		Project:         projectRef.Id,
		GithubPatchData: thirdparty.GithubPatch{HeadOwner: "fork-owner", Author: "pr-author"},
	}
	mfst := &manifest.Manifest{Modules: map[string]*manifest.Module{
		"self":   {Owner: "myorg", Repo: "myrepo"},
		"public": {Owner: "someorg", Repo: "public-repo"},
		"pr":     {Owner: "secretorg", Repo: "pr-readable"},
	}}

	require.NoError(t, newPatchModuleRepoChecker(projectRef, p).recordTrust(t.Context(), mfst))
	assert.Equal(t, manifest.ModuleRepoTrustCommitted, mfst.Modules["self"].RepoTrust)
	assert.Equal(t, manifest.ModuleRepoTrustPublic, mfst.Modules["public"].RepoTrust)
	assert.Equal(t, manifest.ModuleRepoTrustPRAuthor, mfst.Modules["pr"].RepoTrust)
}

func TestTaskRepoAccess(t *testing.T) {
	settings := testutil.TestConfig()
	collections := []string{
		task.Collection, ProjectRefCollection,
		VersionCollection, ParserProjectCollection,
		manifest.Collection, patch.Collection,
	}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})
	githubCalls := stubModuleRepoGitHubChecks(t, nil, nil)

	pRef := ProjectRef{Id: "p1", Owner: "myorg", Repo: "myrepo"}
	v := Version{Id: "v1", Identifier: "p1", Revision: "abc123", Requester: evergreen.RepotrackerVersionRequester}
	pp := ParserProject{
		Id: "v1",
		Modules: []Module{
			{Name: "mod1", Owner: "myorg", Repo: "mod-repo"},
			{Name: "mod2", Owner: "otherorg", Repo: "other-repo"},
		},
	}
	mainlineTask := task.Task{Id: "t1", Project: "p1", Version: "v1", Requester: evergreen.RepotrackerVersionRequester}
	require.NoError(t, pRef.Insert(t.Context()))
	require.NoError(t, v.Insert(t.Context()))
	require.NoError(t, pp.Insert(t.Context()))

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
	require.NoError(t, mainlineManifest.Insert(t.Context()))

	// This patch was finalized before trust was recorded in manifests.
	oldPatch := patch.Patch{Id: mgobson.NewObjectId(), Project: "p1", Githash: "abc123"}
	require.NoError(t, oldPatch.Insert(t.Context()))
	oldPatchManifest := manifest.Manifest{
		Id:          oldPatch.Id.Hex(),
		ProjectName: "p1",
		Modules: map[string]*manifest.Module{
			"mod1":   {Owner: "myorg", Repo: "mod-repo"},
			"secret": {Owner: "secretorg", Repo: "secret-repo"},
		},
	}
	require.NoError(t, oldPatchManifest.Insert(t.Context()))
	oldPatchTask := task.Task{Id: "old-patch-task", Project: "p1", Version: oldPatch.Id.Hex(), Requester: evergreen.PatchVersionRequester}

	recordedPatchManifest := manifest.Manifest{
		Id:          mgobson.NewObjectId().Hex(),
		ProjectName: "p1",
		Modules: map[string]*manifest.Module{
			"mod1":      {Owner: "myorg", Repo: "mod-repo", RepoTrust: manifest.ModuleRepoTrustCommitted},
			"pr":        {Owner: "secretorg", Repo: "pr-readable", RepoTrust: manifest.ModuleRepoTrustPRAuthor},
			"untrusted": {Owner: "secretorg", Repo: "secret-repo", RepoTrust: manifest.ModuleRepoTrustUntrusted},
		},
	}
	require.NoError(t, recordedPatchManifest.Insert(t.Context()))
	recordedPatchTask := task.Task{Id: "recorded-patch-task", Project: "p1", Version: recordedPatchManifest.Id, Requester: evergreen.GithubPRRequester}

	for name, tc := range map[string]struct {
		task    *task.Task
		forUser bool
		owner   string
		repo    string
		allowed bool
	}{
		"ProjectRepoIsAllowed":                      {task: &mainlineTask, owner: "myorg", repo: "myrepo", allowed: true},
		"ProjectRepoIsCaseInsensitive":              {task: &mainlineTask, owner: "MyOrg", repo: "MyRepo", allowed: true},
		"DeclaredModuleIsAllowed":                   {task: &mainlineTask, owner: "myorg", repo: "mod-repo", allowed: true},
		"DeclaredModuleDifferentOrgIsAllowed":       {task: &mainlineTask, owner: "otherorg", repo: "other-repo", allowed: true},
		"UndeclaredRepoIsRejected":                  {task: &mainlineTask, owner: "myorg", repo: "secret-repo"},
		"UndeclaredOwnerIsRejected":                 {task: &mainlineTask, owner: "attacker", repo: "myrepo"},
		"OldPatchModuleInMainlineManifestIsAllowed": {task: &oldPatchTask, owner: "myorg", repo: "mod-repo", allowed: true},
		"OldPatchModuleNotInMainlineIsRejected":     {task: &oldPatchTask, owner: "secretorg", repo: "secret-repo"},
		"RecordedCommittedModuleIsAllowed":          {task: &recordedPatchTask, owner: "myorg", repo: "mod-repo", allowed: true},
		"RecordedPRAuthorModuleIsAllowedForTask":    {task: &recordedPatchTask, owner: "secretorg", repo: "pr-readable", allowed: true},
		"RecordedPRAuthorModuleIsRejectedForUser":   {task: &recordedPatchTask, forUser: true, owner: "secretorg", repo: "pr-readable"},
		"RecordedUntrustedModuleIsRejected":         {task: &recordedPatchTask, owner: "secretorg", repo: "secret-repo"},
		"RecordedCommittedModuleIsAllowedForUser":   {task: &recordedPatchTask, forUser: true, owner: "myorg", repo: "mod-repo", allowed: true},
	} {
		t.Run(name, func(t *testing.T) {
			access, err := NewTaskRepoAccess(t.Context(), settings, tc.task, tc.forUser)
			require.NoError(t, err)
			allowed, err := access.IsAllowed(t.Context(), tc.owner, tc.repo)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, allowed)
		})
	}

	t.Run("RecordedTrustDoesNotCallGitHub", func(t *testing.T) {
		*githubCalls = 0
		access, err := NewTaskRepoAccess(t.Context(), settings, &recordedPatchTask, false)
		require.NoError(t, err)
		for _, repo := range []string{"mod-repo", "pr-readable", "secret-repo"} {
			_, err := access.IsAllowed(t.Context(), "secretorg", repo)
			require.NoError(t, err)
		}
		assert.Zero(t, *githubCalls)
	})
}
