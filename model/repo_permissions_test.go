package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplacingHiddenProjectUpdatesRepoAndRestrictionScopes(t *testing.T) {
	for _, tc := range []struct {
		name              string
		attached          bool
		repoRestricted    bool
		projectRestricted *bool
	}{
		{name: "StandaloneProjectLosesRepoAccess"},
		{name: "StandaloneRestrictedProjectLosesUnrestrictedAccess", projectRestricted: utility.TruePtr()},
		{name: "AttachedRestrictedProjectLosesRepoEditorAccess", attached: true, projectRestricted: utility.TruePtr()},
		{name: "AttachedProjectKeepsRepoAccess", attached: true},
		{name: "AttachedProjectInheritsRepoRestriction", attached: true, repoRestricted: true},
		{name: "AttachedProjectOverridesRepoRestriction", attached: true, repoRestricted: true, projectRestricted: utility.FalsePtr()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			collections := []string{ProjectRefCollection, RepoRefCollection, ProjectVarsCollection, evergreen.ScopeCollection, evergreen.RoleCollection}
			require.NoError(t, db.ClearCollections(collections...))
			t.Cleanup(func() { require.NoError(t, db.ClearCollections(collections...)) })
			require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
			rm := evergreen.GetEnvironment().RoleManager()
			for _, scopeID := range []string{evergreen.UnrestrictedProjectsScope, evergreen.RestrictedProjectsScope} {
				require.NoError(t, rm.AddScope(ctx, gimlet.Scope{ID: scopeID, Type: evergreen.ProjectResourceType, Resources: []string{}}))
			}
			repo := &RepoRef{ProjectRef: ProjectRef{Id: "repo", Owner: "owner", Repo: "repo", Restricted: &tc.repoRestricted}}
			require.NoError(t, repo.Add(ctx, nil))
			hidden := &ProjectRef{Id: "hidden", Owner: repo.Owner, Repo: repo.Repo, Branch: "branch", RepoRefId: repo.Id, Hidden: utility.TruePtr()}
			require.NoError(t, hidden.Add(ctx, nil))
			editorRole := gimlet.Role{ID: "repoEditor", Scope: GetUnrestrictedBranchProjectsScope(repo.Id), Permissions: gimlet.Permissions{evergreen.PermissionTasks: evergreen.TasksBasic.Value}}
			require.NoError(t, rm.UpdateRole(ctx, editorRole))
			editor := &user.DBUser{SystemRoles: []string{editorRole.ID}}
			admin := &user.DBUser{SystemRoles: []string{GetRepoAdminRole(repo.Id)}}
			permission := gimlet.PermissionOpts{Resource: hidden.Id, ResourceType: evergreen.ProjectResourceType, Permission: evergreen.PermissionTasks, RequiredLevel: evergreen.TasksBasic.Value}
			assert.True(t, admin.HasPermission(ctx, permission))
			assert.Equal(t, !tc.repoRestricted, editor.HasPermission(ctx, permission))

			project := &ProjectRef{Id: "newID", Identifier: "tracked", Owner: repo.Owner, Repo: repo.Repo, Branch: hidden.Branch, Restricted: tc.projectRestricted}
			if tc.attached {
				project.RepoRefId = repo.Id
			}
			require.NoError(t, project.Add(ctx, nil))
			assert.Equal(t, hidden.Id, project.Id)
			stored, err := FindBranchProjectRef(ctx, project.Id)
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.False(t, stored.IsHidden())
			assert.Equal(t, project.RepoRefId, stored.RepoRefId)
			restricted := utility.FromBoolPtr(tc.projectRestricted)
			if tc.attached && tc.projectRestricted == nil {
				restricted = tc.repoRestricted
			}
			assert.Equal(t, tc.attached, admin.HasPermission(ctx, permission))
			assert.Equal(t, tc.attached && !restricted, editor.HasPermission(ctx, permission))
			for scopeID, shouldContain := range map[string]bool{
				evergreen.UnrestrictedProjectsScope: !restricted,
				evergreen.RestrictedProjectsScope:   restricted,
			} {
				scope, err := rm.GetScope(ctx, scopeID)
				require.NoError(t, err)
				require.NotNil(t, scope)
				assert.Equal(t, shouldContain, utility.StringSliceContains(scope.Resources, project.Id))
			}
		})
	}
}

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
