package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoPermissionScopesRespectInheritedRestrictions(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	rm := evergreen.GetEnvironment().RoleManager()
	repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo", Restricted: utility.TruePtr()}}
	require.NoError(t, repo.Add(ctx, nil))
	projects := []ProjectRef{
		{Id: "inheritsRestriction", RepoRefId: repo.Id},
		{Id: "explicitlyUnrestricted", RepoRefId: repo.Id, Restricted: utility.FalsePtr()},
		{Id: "explicitlyRestricted", RepoRefId: repo.Id, Restricted: utility.TruePtr()},
		{Id: "notAttached", Owner: repo.Owner, Repo: repo.Repo},
	}
	for _, project := range projects {
		require.NoError(t, project.Insert(ctx))
	}
	// Repair the stale membership that existed before repo entitlements.
	require.NoError(t, rm.AddResourceToScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id), "inheritsRestriction"))
	require.NoError(t, EnsureRepoPermissionScopes(ctx, rm, repo))
	scope, err := rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.ElementsMatch(t, []string{"explicitlyUnrestricted"}, scope.Resources)
	adminScope, err := rm.GetScope(ctx, GetRepoAdminScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, adminScope)
	assert.ElementsMatch(t, []string{repo.Id, "inheritsRestriction", "explicitlyRestricted", "explicitlyUnrestricted"}, adminScope.Resources)
	settingsScope, err := rm.GetScope(ctx, GetRepoOnlyScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, settingsScope)
	assert.Equal(t, []string{repo.Id}, settingsScope.Resources)

	// Restriction changes preserve explicit branch overrides.
	repo.Restricted = utility.FalsePtr()
	branches, err := FindMergedProjectRefsForRepo(ctx, repo)
	require.NoError(t, err)
	require.NoError(t, repo.MakeUnrestricted(ctx, branches))
	scope, err = rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.ElementsMatch(t, []string{"explicitlyUnrestricted", "inheritsRestriction"}, scope.Resources)
	repo.Restricted = utility.TruePtr()
	branches, err = FindMergedProjectRefsForRepo(ctx, repo)
	require.NoError(t, err)
	require.NoError(t, repo.MakeRestricted(ctx, branches))
	scope, err = rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.ElementsMatch(t, []string{"explicitlyUnrestricted"}, scope.Resources)
	project := projects[0]
	require.NoError(t, project.RemoveFromRepoScope(ctx))
	scope, err = rm.GetScope(ctx, GetUnrestrictedBranchProjectsScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, scope)
	assert.NotContains(t, scope.Resources, project.Id)
}

func TestNewUntrackedBranchesInheritRepoRestrictionsInScopes(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, evergreen.ScopeCollection, evergreen.RoleCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	rm := evergreen.GetEnvironment().RoleManager()
	repo := &RepoRef{ProjectRef: ProjectRef{
		Id: "repo", Owner: "owner", Repo: "repo", Restricted: utility.TruePtr(),
		RemotePath: "evergreen.yml", PRTestingEnabled: utility.TruePtr(),
	}}
	require.NoError(t, repo.Add(ctx, nil))
	project, err := FindOneProjectRefByRepoAndBranchWithPRTesting(ctx, repo.Owner, repo.Repo, "untracked", patch.AutomatedCaller)
	require.NoError(t, err)
	require.NotNil(t, project)
	assert.True(t, project.IsHidden())
	merged, err := GetProjectRefMergedWithRepo(ctx, *project)
	require.NoError(t, err)
	require.NotNil(t, merged)
	assert.True(t, merged.IsRestricted())
	for _, scopeID := range []string{GetUnrestrictedBranchProjectsScope(repo.Id), evergreen.UnrestrictedProjectsScope} {
		scope, err := rm.GetScope(ctx, scopeID)
		require.NoError(t, err)
		if scope != nil {
			assert.NotContains(t, scope.Resources, project.Id)
		}
	}
	adminScope, err := rm.GetScope(ctx, GetRepoAdminScope(repo.Id))
	require.NoError(t, err)
	require.NotNil(t, adminScope)
	assert.Contains(t, adminScope.Resources, project.Id)
}
