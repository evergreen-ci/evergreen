package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePatchModuleRepos(t *testing.T) {
	collections := []string{ProjectRefCollection, VersionCollection, manifest.Collection, ProjectVarsCollection}
	require.NoError(t, db.ClearCollections(collections...))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(collections...))
	})

	origPublic, origRead := isGitHubRepoPublic, gitHubUserHasReadPermission
	t.Cleanup(func() {
		isGitHubRepoPublic, gitHubUserHasReadPermission = origPublic, origRead
	})
	isGitHubRepoPublic = func(_ context.Context, owner, repo string) (bool, error) {
		if owner+"/"+repo == "secretorg/github-error" {
			return false, errors.New("GitHub is down")
		}
		return owner+"/"+repo == "someorg/public-repo", nil
	}
	gitHubUserHasReadPermission = func(_ context.Context, owner, repo, username string) (bool, error) {
		return username == "pr-author" && owner+"/"+repo == "secretorg/pr-readable", nil
	}

	projectRef := &ProjectRef{Id: "trust-project", Owner: "myorg", Repo: "myrepo", RepoRefId: "repo-ref"}
	for _, ref := range []ProjectRef{
		*projectRef,
		{Id: "same-repo-ref", Owner: "myorg", Repo: "renamed", RepoRefId: "repo-ref"},
		{Id: "same-owner-repo", Owner: "myorg", Repo: "myrepo"},
		{Id: "hidden", Owner: "myorg", Repo: "myrepo", Hidden: utility.TruePtr()},
		{Id: "other-repo", Owner: "myorg", Repo: "other"},
	} {
		require.NoError(t, ref.Insert(t.Context()))
	}
	for i, v := range []Version{
		{Id: "base", Identifier: projectRef.Id, Revision: "base-revision", Requester: evergreen.RepotrackerVersionRequester},
		{Id: "recent", Identifier: projectRef.Id, Requester: evergreen.RepotrackerVersionRequester},
		{Id: "tag", Identifier: projectRef.Id, Requester: evergreen.GitTagRequester},
		{Id: "same-repo-ref", Identifier: "same-repo-ref", Requester: evergreen.RepotrackerVersionRequester},
		{Id: "same-owner-repo", Identifier: "same-owner-repo", Requester: evergreen.RepotrackerVersionRequester},
		{Id: "hidden", Identifier: "hidden", Requester: evergreen.RepotrackerVersionRequester},
		{Id: "other-repo", Identifier: "other-repo", Requester: evergreen.RepotrackerVersionRequester},
	} {
		v.RevisionOrderNumber = i + 1
		require.NoError(t, v.Insert(t.Context()))
	}
	for _, m := range []manifest.Manifest{
		{Id: "base", ProjectName: projectRef.Id, Revision: "base-revision", IsBase: true, Modules: map[string]*manifest.Module{"base": {Owner: "myorg", Repo: "base-module"}}},
		{Id: "recent", ProjectName: projectRef.Id, Modules: map[string]*manifest.Module{"recent": {Owner: "otherorg", Repo: "recent-module"}}},
		{Id: "tag", ProjectName: projectRef.Id, Modules: map[string]*manifest.Module{"tag": {Owner: "secretorg", Repo: "tagged-repo"}}},
		{Id: "same-repo-ref", Modules: map[string]*manifest.Module{"sibling": {Owner: "secretorg", Repo: "repo-ref-sibling"}}},
		{Id: "same-owner-repo", Modules: map[string]*manifest.Module{"sibling": {Owner: "secretorg", Repo: "owner-repo-sibling"}, "flaky": {Owner: "secretorg", Repo: "github-error"}}},
		{Id: "hidden", Modules: map[string]*manifest.Module{"sibling": {Owner: "secretorg", Repo: "hidden-sibling"}}},
		{Id: "other-repo", Modules: map[string]*manifest.Module{"sibling": {Owner: "secretorg", Repo: "other-repo-sibling"}}},
	} {
		require.NoError(t, m.Insert(t.Context()))
	}
	_, err := (&ProjectVars{Id: projectRef.Id, Vars: map[string]string{"owner": "myorg"}}).Upsert(t.Context())
	require.NoError(t, err)

	cliPatch := &patch.Patch{Id: mgobson.NewObjectId(), Project: projectRef.Id, Githash: "base-revision"}
	prPatch := &patch.Patch{
		Id:              mgobson.NewObjectId(),
		Project:         projectRef.Id,
		GithubPatchData: thirdparty.GithubPatch{HeadOwner: "fork-owner", Author: "pr-author"},
	}
	secret := Module{Name: "secret", Owner: "secretorg", Repo: "secret-repo", Branch: "main"}
	prReadable := Module{Name: "pr", Owner: "secretorg", Repo: "pr-readable"}

	t.Run("CommittedAndPublicReposAreAllowed", func(t *testing.T) {
		assert.NoError(t, ValidatePatchModuleRepos(t.Context(), cliPatch, projectRef, ModuleList{
			{Name: "self", Repo: "git@github.com:myorg/myrepo.git"},
			{Name: "base", Owner: "myorg", Repo: "base-module"},
			{Name: "recent", Owner: "OtherOrg", Repo: "recent-module.git"},
			{Name: "wiki", Owner: "myorg", Repo: "base-module.wiki"},
			{Name: "expanded", Owner: "${owner}", Repo: "base-module"},
			{Name: "public", Owner: "someorg", Repo: "public-repo"},
			{Name: "repo-ref-sibling", Owner: "secretorg", Repo: "repo-ref-sibling"},
			{Name: "owner-repo-sibling", Owner: "secretorg", Repo: "owner-repo-sibling"},
			{Name: "flaky", Owner: "secretorg", Repo: "github-error"},
		}))
	})
	for name, tc := range map[string]struct {
		p      *patch.Patch
		module Module
	}{
		"UndeclaredPrivateRepoIsRejected":       {p: cliPatch, module: secret},
		"ModuleRetargetedToNewRepoIsRejected":   {p: cliPatch, module: Module{Name: "base", Owner: "secretorg", Repo: "secret-repo"}},
		"RepoOnlyInGitTagVersionIsRejected":     {p: cliPatch, module: Module{Name: "tag", Owner: "secretorg", Repo: "tagged-repo"}},
		"RepoOnlyInHiddenProjectIsRejected":     {p: cliPatch, module: Module{Name: "hidden", Owner: "secretorg", Repo: "hidden-sibling"}},
		"RepoOnlyInOtherReposProjectIsRejected": {p: cliPatch, module: Module{Name: "other", Owner: "secretorg", Repo: "other-repo-sibling"}},
		"PRAuthorAccessIsRejectedForCLIPatch":   {p: cliPatch, module: prReadable},
		"CLIPatchWithMergeQueueAliasIsRejected": {
			p:      &patch.Patch{Id: mgobson.NewObjectId(), Project: projectRef.Id, Alias: evergreen.CommitQueueAlias},
			module: secret,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidatePatchModuleRepos(t.Context(), tc.p, projectRef, ModuleList{tc.module})
			assert.ErrorAs(t, err, &untrustedModuleReposError{})
			assert.Contains(t, err.Error(), tc.module.Name)
		})
	}
	t.Run("GitHubErrorIsReturnedForUndeclaredRepo", func(t *testing.T) {
		otherPatch := &patch.Patch{Id: mgobson.NewObjectId(), Project: "other-repo"}
		err := ValidatePatchModuleRepos(t.Context(), otherPatch, &ProjectRef{Id: "other-repo", Owner: "myorg", Repo: "other"}, ModuleList{{Name: "flaky", Owner: "secretorg", Repo: "github-error"}})
		require.Error(t, err)
		assert.False(t, IsUntrustedModuleReposError(err))
		assert.Contains(t, err.Error(), "GitHub is down")
	})
	t.Run("PRAuthorAccessIsAllowedForPRPatch", func(t *testing.T) {
		assert.NoError(t, ValidatePatchModuleRepos(t.Context(), prPatch, projectRef, ModuleList{prReadable}))
	})
	t.Run("ReviewedConfigIsNotChecked", func(t *testing.T) {
		mergeQueuePatch := &patch.Patch{Id: mgobson.NewObjectId(), GithubMergeData: thirdparty.GithubMergeGroup{HeadSHA: "sha"}}
		childPatch := &patch.Patch{Id: mgobson.NewObjectId(), Triggers: patch.TriggerInfo{ParentPatch: "parent"}}
		assert.NoError(t, ValidatePatchModuleRepos(t.Context(), mergeQueuePatch, projectRef, ModuleList{secret}))
		assert.NoError(t, ValidatePatchModuleRepos(t.Context(), childPatch, projectRef, ModuleList{secret}))
	})
	t.Run("IncludeFromUntrustedRepoIsRejected", func(t *testing.T) {
		opts := GetProjectOpts{Ref: projectRef, ReadFileFrom: ReadFromPatch, moduleRepoChecker: newPatchModuleRepoChecker(projectRef, cliPatch)}
		_, err := retrieveFileForModule(t.Context(), opts, ModuleList{secret}, parserInclude{FileName: "include.yml", Module: secret.Name}, nil, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "secretorg/secret-repo")
	})
}
