package route

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoGrantsAndProjectRevocationAreIndependent(t *testing.T) {
	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.RepoRefCollection, user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	repo := &model.RepoRef{ProjectRef: model.ProjectRef{Id: "repo", Owner: "owner", Repo: "repo"}}
	require.NoError(t, repo.Add(ctx, nil))
	for _, id := range []string{"a", "b"} {
		require.NoError(t, (&model.ProjectRef{Id: id, RepoRefId: repo.Id}).Insert(ctx))
	}
	u := &user.DBUser{Id: "editor"}
	require.NoError(t, u.Insert(ctx))
	editorPermissions := gimlet.Permissions{evergreen.PermissionTasks: 20, evergreen.PermissionPatches: 10, evergreen.PermissionLogs: 10}
	grant := func(request RequestedPermissions) string {
		h := &userPermissionsPostHandler{rm: rm, userID: u.Id, permissions: request}
		response := h.Run(ctx)
		require.Equal(t, http.StatusOK, response.Status(), response.Data())
		current, err := user.FindOneById(ctx, u.Id)
		require.NoError(t, err)
		require.NotNil(t, current)
		return current.SystemRoles[len(current.SystemRoles)-1]
	}
	repoRoleID := grant(RequestedPermissions{ResourceType: repoResourceType, Resources: []string{repo.Id}, RepoScope: model.RepoUnrestrictedScope, Permissions: editorPermissions})
	projectRoleID := grant(RequestedPermissions{ResourceType: evergreen.ProjectResourceType, Resources: []string{"a"}, Permissions: editorPermissions})
	require.NotEqual(t, repoRoleID, projectRoleID)
	projectRevoke := &userPermissionsDeleteHandler{rm: rm, userID: u.Id, resourceType: evergreen.ProjectResourceType, resourceId: "a"}
	require.Equal(t, http.StatusOK, projectRevoke.Run(ctx).Status())
	current, err := user.FindOneById(ctx, u.Id)
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Contains(t, current.Roles(), repoRoleID)
	assert.NotContains(t, current.Roles(), projectRoleID)
	assert.True(t, current.HasPermission(ctx, gimlet.PermissionOpts{Resource: "b", ResourceType: evergreen.ProjectResourceType, Permission: evergreen.PermissionTasks, RequiredLevel: 20}))
	projectRoleID = grant(RequestedPermissions{ResourceType: evergreen.ProjectResourceType, Resources: []string{"a"}, Permissions: editorPermissions})
	require.NoError(t, current.AddRole(ctx, model.GetRepoAdminRole(repo.Id)))
	repoRevoke := &userPermissionsDeleteHandler{rm: rm, userID: u.Id, resourceType: repoResourceType, resourceId: repo.Id}
	require.Equal(t, http.StatusOK, repoRevoke.Run(ctx).Status())
	current, err = user.FindOneById(ctx, u.Id)
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Contains(t, current.Roles(), projectRoleID)
	assert.Contains(t, current.Roles(), model.GetRepoAdminRole(repo.Id))
	assert.NotContains(t, current.Roles(), repoRoleID)
	// Revocation still works when no unrestricted branch resources remain.
	for _, id := range []string{"a", "b"} {
		require.NoError(t, rm.RemoveResourceFromScope(ctx, model.GetUnrestrictedBranchProjectsScope(repo.Id), id))
	}
	emptyScopeUser := &user.DBUser{Id: "emptyScopeEditor", SystemRoles: []string{repoRoleID}}
	require.NoError(t, emptyScopeUser.Insert(ctx))
	response := (&userPermissionsDeleteHandler{rm: rm, userID: emptyScopeUser.Id, resourceType: repoResourceType, resourceId: repo.Id}).Run(ctx)
	require.Equal(t, http.StatusOK, response.Status())
	emptyScopeUser, err = user.FindOneById(ctx, emptyScopeUser.Id)
	require.NoError(t, err)
	require.NotNil(t, emptyScopeUser)
	assert.Empty(t, emptyScopeUser.Roles())
	// A role stays useful for branches added after the initial grant.
	require.NoError(t, rm.AddResourceToScope(ctx, model.GetUnrestrictedBranchProjectsScope(repo.Id), "future"))
	u2 := &user.DBUser{Id: "newEditor"}
	require.NoError(t, u2.Insert(ctx))
	require.NoError(t, u2.AddRole(ctx, repoRoleID))
	assert.True(t, u2.HasPermission(ctx, gimlet.PermissionOpts{Resource: "future", ResourceType: evergreen.ProjectResourceType, Permission: evergreen.PermissionTasks, RequiredLevel: 20}))
}

func TestRepoGrantsDoNotReuseBroaderRolesOrMatchingProjectScopes(t *testing.T) {
	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.RepoRefCollection, user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	repo := &model.RepoRef{ProjectRef: model.ProjectRef{Id: "repo"}}
	require.NoError(t, repo.Add(ctx, nil))
	request := RequestedPermissions{ResourceType: repoResourceType, Resources: []string{repo.Id}, RepoScope: model.RepoOnlyScope, Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: 20}}
	role, err := makeRepoPermissionRole(ctx, rm, request)
	require.NoError(t, err)
	require.NotNil(t, role)
	assert.NotEqual(t, model.GetRepoAdminRole(repo.Id), role.ID)
	assert.Equal(t, model.GetRepoOnlyScope(repo.Id), role.Scope)
	assert.Equal(t, request.Permissions, role.Permissions)
	u := &user.DBUser{Id: "settings", SystemRoles: []string{role.ID}}
	require.NoError(t, u.Insert(ctx))
	canView, err := model.UserHasRepoViewPermission(ctx, u, repo.Id)
	require.NoError(t, err)
	assert.True(t, canView)
	require.NoError(t, (&model.ProjectRef{Id: "branch", RepoRefId: repo.Id}).Insert(ctx))
	assert.False(t, u.HasPermission(ctx, gimlet.PermissionOpts{Resource: "branch", ResourceType: evergreen.ProjectResourceType, Permission: evergreen.PermissionProjectSettings, RequiredLevel: evergreen.ProjectSettingsEdit.Value}))
	projectRole, err := makeProjectPermissionRole(ctx, rm, RequestedPermissions{ResourceType: evergreen.ProjectResourceType, Resources: []string{repo.Id}, Permissions: request.Permissions})
	require.NoError(t, err)
	require.NotNil(t, projectRole)
	assert.False(t, model.IsRepoPermissionScope(projectRole.Scope))
}

func TestRepoUsersIncludeEachCapabilityAndExcludeProjectGrants(t *testing.T) {
	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.RepoRefCollection, user.Collection, evergreen.RoleCollection, evergreen.ScopeCollection))
	require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))
	repo := &model.RepoRef{ProjectRef: model.ProjectRef{Id: "repo"}}
	require.NoError(t, repo.Add(ctx, nil))
	require.NoError(t, (&model.ProjectRef{Id: "branch", RepoRefId: repo.Id}).Insert(ctx))
	for _, tc := range []struct {
		id          string
		scope       model.RepoPermissionScope
		permissions gimlet.Permissions
	}{
		{"settings", model.RepoOnlyScope, gimlet.Permissions{evergreen.PermissionProjectSettings: 20}},
		{"editor", model.RepoUnrestrictedScope, gimlet.Permissions{evergreen.PermissionTasks: 20, evergreen.PermissionPatches: 10, evergreen.PermissionLogs: 10}},
		{"admin", model.RepoAndProjectsScope, gimlet.Permissions{evergreen.PermissionProjectSettings: 20, evergreen.PermissionTasks: 30, evergreen.PermissionPatches: 10, evergreen.PermissionLogs: 10}},
	} {
		role, err := makeRepoPermissionRole(ctx, rm, RequestedPermissions{ResourceType: repoResourceType, Resources: []string{repo.Id}, RepoScope: tc.scope, Permissions: tc.permissions})
		require.NoError(t, err)
		require.NoError(t, (&user.DBUser{Id: tc.id, SystemRoles: []string{role.ID}}).Insert(ctx))
	}
	require.NoError(t, (&user.DBUser{Id: "native", SystemRoles: []string{model.GetRepoAdminRole(repo.Id)}}).Insert(ctx))
	require.NoError(t, (&user.DBUser{Id: "bot", OnlyAPI: true, SystemRoles: []string{model.GetRepoAdminRole(repo.Id)}}).Insert(ctx))
	role, err := makeProjectPermissionRole(ctx, rm, RequestedPermissions{ResourceType: evergreen.ProjectResourceType, Resources: []string{"branch"}, Permissions: gimlet.Permissions{evergreen.PermissionTasks: 30}})
	require.NoError(t, err)
	require.NoError(t, (&user.DBUser{Id: "projectOnly", SystemRoles: []string{role.ID}}).Insert(ctx))
	response := getRepoUserPermissions(ctx, rm, repo.Id)
	require.Equal(t, http.StatusOK, response.Status())
	result := response.Data().(RepoUserPermissions)
	assert.Len(t, result, 4)
	assert.Contains(t, result["settings"], model.RepoOnlyScope)
	assert.Contains(t, result["editor"], model.RepoUnrestrictedScope)
	assert.Contains(t, result["admin"], model.RepoAndProjectsScope)
	assert.Contains(t, result["native"], model.RepoAndProjectsScope)
	assert.NotContains(t, result, "projectOnly")
	assert.NotContains(t, result, "bot")
	projectResult := (&allUsersPermissionsGetHandler{rm: rm, input: UsersPermissionsInput{
		ResourceType: evergreen.ProjectResourceType, ResourceId: "branch", ExcludeRepoGrants: true,
	}}).Run(ctx)
	require.Equal(t, http.StatusOK, projectResult.Status())
	assert.Equal(t, UsersPermissionsResult{"projectOnly": {evergreen.PermissionTasks: 30}}, projectResult.Data())
}

func TestRepoDiscoveryReturnsOnlyResourceMetadata(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.RepoRefCollection))
	repo := &model.RepoRef{ProjectRef: model.ProjectRef{Id: "repo", Owner: "owner", Repo: "name", Admins: []string{"admin"}, RemotePath: "evergreen.yml"}}
	require.NoError(t, repo.Replace(t.Context()))
	response := (&reposGetHandler{}).Run(t.Context())
	require.Equal(t, http.StatusOK, response.Status())
	encoded, err := json.Marshal(response.Data())
	require.NoError(t, err)
	assert.JSONEq(t, `[{"id":"repo","name":"owner/name","admins":["admin"]}]`, string(encoded))
}

func TestRepoPermissionRequestsRejectInvalidCapabilities(t *testing.T) {
	rm := testutil.NewEnvironment(t.Context(), t).RoleManager()
	for _, tc := range []struct {
		name    string
		request RequestedPermissions
	}{
		{"MissingScope", RequestedPermissions{ResourceType: repoResourceType, Resources: []string{"repo"}}},
		{"MultipleRepos", RequestedPermissions{ResourceType: repoResourceType, Resources: []string{"a", "b"}, RepoScope: model.RepoOnlyScope}},
		{"SettingsScopeWithTaskAdmin", RequestedPermissions{ResourceType: repoResourceType, Resources: []string{"a"}, RepoScope: model.RepoOnlyScope, Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: 20, evergreen.PermissionTasks: 30}}},
		{"ScopeOnProjectGrant", RequestedPermissions{ResourceType: evergreen.ProjectResourceType, Resources: []string{"a"}, RepoScope: model.RepoOnlyScope}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.request)
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, "/users/u/permissions", bytes.NewReader(body))
			require.NoError(t, err)
			req = gimlet.SetURLVars(req, map[string]string{"user_id": "u"})
			assert.Error(t, (&userPermissionsPostHandler{rm: rm}).Parse(t.Context(), req))
		})
	}
	assert.NoError(t, validateRepoPermissions(model.RepoOnlyScope, gimlet.Permissions{evergreen.PermissionProjectSettings: 20, evergreen.PermissionTasks: 0}))
}
