package graphql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
)

func init() {
	testutil.Setup()
}

func setupPermissions(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))

	roleManager := env.RoleManager()

	roles, err := roleManager.GetAllRoles()
	require.NoError(t, err)
	require.Empty(t, roles)

	superUserRole := gimlet.Role{
		ID:    "superuser",
		Name:  "superuser",
		Scope: "superuser_scope",
		Permissions: map[string]int{
			evergreen.PermissionProjectCreate: evergreen.ProjectCreate.Value,
			evergreen.PermissionDistroCreate:  evergreen.DistroCreate.Value,
			evergreen.PermissionRoleModify:    evergreen.RoleModify.Value,
			evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value,
		},
	}
	err = roleManager.UpdateRole(superUserRole)
	require.NoError(t, err)

	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{"super_user"},
	}
	err = roleManager.AddScope(superUserScope)
	require.NoError(t, err)

	superUserProjectRole := gimlet.Role{
		ID:    evergreen.SuperUserProjectAccessRole,
		Name:  "admin access",
		Scope: evergreen.AllProjectsScope,
		Permissions: map[string]int{
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmitAdmin.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			evergreen.PermissionAnnotations:     evergreen.AnnotationsModify.Value,
		},
	}
	require.NoError(t, roleManager.UpdateRole(superUserProjectRole))

	superUserProjectScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Name:      "all projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"project_id"},
	}
	require.NoError(t, roleManager.AddScope(superUserProjectScope))

	superUserDistroRole := gimlet.Role{
		ID:    evergreen.SuperUserDistroAccessRole,
		Name:  "admin access",
		Scope: evergreen.AllDistrosScope,
		Permissions: map[string]int{
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
			evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
		},
	}
	require.NoError(t, roleManager.UpdateRole(superUserDistroRole))

	superUserDistroScope := gimlet.Scope{
		ID:        evergreen.AllDistrosScope,
		Name:      "all distros",
		Type:      evergreen.DistroResourceType,
		Resources: []string{"distro-id"},
	}
	require.NoError(t, roleManager.AddScope(superUserDistroScope))

	projectScope := gimlet.Scope{
		ID:        "project_scope",
		Name:      "project scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"project_id"},
	}
	err = roleManager.AddScope(projectScope)
	require.NoError(t, err)

	projectAdminRole := gimlet.Role{
		ID:    "admin_project",
		Scope: projectScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmitAdmin.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
		},
	}
	err = roleManager.UpdateRole(projectAdminRole)
	require.NoError(t, err)

	projectViewRole := gimlet.Role{
		ID:    "view_project",
		Scope: projectScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
		},
	}
	err = roleManager.UpdateRole(projectViewRole)
	require.NoError(t, err)

	distroScope := gimlet.Scope{
		ID:        "distro_distro-id",
		Name:      "distro-id",
		Type:      evergreen.DistroResourceType,
		Resources: []string{"distro-id"},
	}
	require.NoError(t, roleManager.AddScope(distroScope))

	distroAdminRole := gimlet.Role{
		ID:          "admin_distro-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value},
	}
	require.NoError(t, roleManager.UpdateRole(distroAdminRole))

	distroEditRole := gimlet.Role{
		ID:          "edit_distro-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionDistroSettings: evergreen.DistroSettingsEdit.Value},
	}
	require.NoError(t, roleManager.UpdateRole(distroEditRole))

	distroViewRole := gimlet.Role{
		ID:          "view_distro-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionDistroSettings: evergreen.DistroSettingsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(distroViewRole))

	hostEditRole := gimlet.Role{
		ID:          "edit_host-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsEdit.Value},
	}
	require.NoError(t, roleManager.UpdateRole(hostEditRole))

	hostViewRole := gimlet.Role{
		ID:          "view_host-id",
		Scope:       distroScope.ID,
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(hostViewRole))

	taskAdminRole := gimlet.Role{
		ID:          "admin_task",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksAdmin.Value},
	}
	require.NoError(t, roleManager.UpdateRole(taskAdminRole))

	taskEditRole := gimlet.Role{
		ID:          "edit_task",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksBasic.Value},
	}
	require.NoError(t, roleManager.UpdateRole(taskEditRole))

	taskViewRole := gimlet.Role{
		ID:          "view_task",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(taskViewRole))

	annotationEditRole := gimlet.Role{
		ID:          "edit_annotation",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionAnnotations: evergreen.AnnotationsModify.Value},
	}
	require.NoError(t, roleManager.UpdateRole(annotationEditRole))

	annotationViewRole := gimlet.Role{
		ID:          "view_annotation",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionAnnotations: evergreen.AnnotationsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(annotationViewRole))

	patchAdminRole := gimlet.Role{
		ID:          "admin_patch",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionPatches: evergreen.PatchSubmitAdmin.Value},
	}
	require.NoError(t, roleManager.UpdateRole(patchAdminRole))

	patchEditRole := gimlet.Role{
		ID:          "edit_patch",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionPatches: evergreen.PatchSubmit.Value},
	}
	require.NoError(t, roleManager.UpdateRole(patchEditRole))

	logViewRole := gimlet.Role{
		ID:          "view_logs",
		Scope:       projectScope.ID,
		Permissions: map[string]int{evergreen.PermissionLogs: evergreen.LogsView.Value},
	}
	require.NoError(t, roleManager.UpdateRole(logViewRole))
}
func TestRequireHostAccess(t *testing.T) {
	defer func() {
		require.NoError(t, db.ClearCollections(host.Collection, user.Collection),
			"unable to clear user or host collection")
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser){
		"FailsWhenHostIdIsNotSpecified": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			obj := any(nil)
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: host not specified")
		},
		"FailsWhenHostDoesNotExist": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			obj := any(map[string]any{"hostId": "a-non-existent-host-id"})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: No matching hosts found")
		},
		"ViewFailsWhenUserDoesNotHaveViewPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			obj := any(map[string]any{"hostId": "host1"})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelView)
			assert.EqualError(t, err, "input: user 'test_user' does not have permission to access host 'host1'")
		},
		"EditFailsWhenUserDoesNotHaveEditPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			obj := any(map[string]any{"hostId": "host1"})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: user 'test_user' does not have permission to access host 'host1'")
		},
		"ViewSucceedsWhenUserHasViewPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			assert.NoError(t, usr.AddRole(ctx, "view_host-id"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := any(map[string]any{"hostId": "host1"})
			res, err := config.Directives.RequireHostAccess(ctx, obj, wrappedNext, HostAccessLevelView)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole(ctx, "view_host-id"))
		},
		"EditSucceedsWhenUserHasEditPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			assert.NoError(t, usr.AddRole(ctx, "edit_host-id"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := any(map[string]any{"hostId": "host1"})
			res, err := config.Directives.RequireHostAccess(ctx, obj, wrappedNext, HostAccessLevelEdit)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole(ctx, "edit_host-id"))
		},
		"ViewSucceedsWhenHostIsStartedByUser": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := any(map[string]any{"hostId": "host2"})
			res, err := config.Directives.RequireHostAccess(ctx, obj, wrappedNext, HostAccessLevelView)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
		},
		"EditSucceedsWhenHostIsStartedByUser": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := any(map[string]any{"hostId": "host2"})
			res, err := config.Directives.RequireHostAccess(ctx, obj, wrappedNext, HostAccessLevelEdit)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			setupPermissions(t)
			usr, err := setupUser(t)
			assert.NoError(t, err)
			assert.NotNil(t, usr)
			ctx = gimlet.AttachUser(ctx, usr)
			assert.NotNil(t, ctx)
			h1 := host.Host{
				Id: "host1",
				Distro: distro.Distro{
					Id: "distro-id",
				},
			}
			assert.NoError(t, h1.Insert(ctx))
			h2 := host.Host{
				Id:        "host2",
				StartedBy: testUser,
				Distro: distro.Distro{
					Id: "distro-id",
				},
			}
			assert.NoError(t, h2.Insert(ctx))
			config := New("/graphql")
			assert.NotNil(t, config)
			next := func(rctx context.Context) (any, error) {
				return nil, nil
			}
			tCase(ctx, t, next, config, usr)
		})
	}
}
func TestRequireDistroAccess(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, user.Collection),
		"unable to clear user or project ref collection")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := any(nil)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := user.GetOrCreateUser(testUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	// Fails when distro is not specified
	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	assert.EqualError(t, err, "input: distro not specified")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessCreate)
	assert.EqualError(t, err, "input: user 'test_user' does not have create distro permissions")

	// superuser should be successful for create with no distro ID specified
	require.NoError(t, usr.AddRole(t.Context(), "superuser"))

	res, err := config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessCreate)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	require.NoError(t, usr.RemoveRole(t.Context(), "superuser"))

	// superuser_distro_access is successful for admin, edit, view
	require.NoError(t, usr.AddRole(t.Context(), "superuser_distro_access"))

	obj = any(map[string]any{"distroId": "distro-id"})
	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 2, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 3, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 4, callCount)

	require.NoError(t, usr.RemoveRole(t.Context(), "superuser_distro_access"))

	// admin access is successful for admin, edit, view
	require.NoError(t, usr.AddRole(t.Context(), "admin_distro-id"))

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 5, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 6, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 7, callCount)

	require.NoError(t, usr.RemoveRole(t.Context(), "admin_distro-id"))

	// edit access fails for admin, is successful for edit & view
	require.NoError(t, usr.AddRole(t.Context(), "edit_distro-id"))

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	assert.Nil(t, res)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to access settings for the distro 'distro-id'")
	assert.Equal(t, 7, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 8, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 9, callCount)

	require.NoError(t, usr.RemoveRole(t.Context(), "edit_distro-id"))

	// view access fails for admin & edit, is successful for view
	require.NoError(t, usr.AddRole(t.Context(), "view_distro-id"))

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	assert.Equal(t, 9, callCount)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to access settings for the distro 'distro-id'")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	assert.Equal(t, 9, callCount)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to access settings for the distro 'distro-id'")

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 10, callCount)

	require.NoError(t, usr.RemoveRole(t.Context(), "view_distro-id"))

	// no access fails all query attempts
	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	assert.Equal(t, 10, callCount)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to access settings for the distro 'distro-id'")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	assert.Equal(t, 10, callCount)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to access settings for the distro 'distro-id'")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	assert.Equal(t, 10, callCount)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to access settings for the distro 'distro-id'")
}

func TestRequireProjectAdmin(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := any(nil)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := user.GetOrCreateUser(testUser, "Mohamed Khelif", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}
	err = projectRef.Insert(t.Context())
	require.NoError(t, err)

	// superuser should always be successful, no matter the resolver
	err = usr.AddRole(t.Context(), "superuser")
	require.NoError(t, err)

	res, err := config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	err = usr.RemoveRole(t.Context(), "superuser")
	require.NoError(t, err)

	// CreateProject - permission denied
	operationContext := &graphql.OperationContext{
		OperationName: CreateProjectMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]any{
		"project": map[string]any{
			"identifier": "anything",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.EqualError(t, err, "input: user test_user does not have permission to access the CreateProject resolver")
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	// CreateProject - successful
	err = usr.AddRole(t.Context(), "admin_project")
	require.NoError(t, err)
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 2, callCount)

	// CopyProject - permission denied
	operationContext = &graphql.OperationContext{
		OperationName: CopyProjectMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]any{
		"project": map[string]any{
			"projectIdToCopy": "anything",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.EqualError(t, err, "input: user test_user does not have permission to access the CopyProject resolver")
	assert.Nil(t, res)
	assert.Equal(t, 2, callCount)

	// CopyProject - successful
	obj = map[string]any{
		"project": map[string]any{
			"projectIdToCopy": "project_id",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 3, callCount)

	// DeleteProject - permission denied
	operationContext = &graphql.OperationContext{
		OperationName: DeleteProjectMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]any{"projectId": "anything"}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.EqualError(t, err, "input: user test_user does not have permission to access the DeleteProject resolver")
	assert.Nil(t, res)
	assert.Equal(t, 3, callCount)

	// DeleteProject - successful
	obj = map[string]any{"projectId": "project_id"}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 4, callCount)

	// SetLastRevision - successful
	operationContext = &graphql.OperationContext{
		OperationName: SetLastRevisionMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]any{
		"opts": map[string]any{
			"projectIdentifier": "project_identifier",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 5, callCount)

	// SetLastRevision - project not found
	operationContext = &graphql.OperationContext{
		OperationName: SetLastRevisionMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]any{
		"opts": map[string]any{
			"projectIdentifier": "project_whatever",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.EqualError(t, err, "input: project 'project_whatever' not found")
	assert.Nil(t, res)
	assert.Equal(t, 5, callCount)

	// SetLastRevision - permission denied
	operationContext = &graphql.OperationContext{
		OperationName: SetLastRevisionMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]any{
		"opts": map[string]any{
			"projectIdentifier": "project_identifier",
		},
	}
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_project"))
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	assert.EqualError(t, err, "input: user test_user does not have permission to access the SetLastRevision resolver")
	assert.Nil(t, res)
	assert.Equal(t, 5, callCount)

}

func setupUser(t *testing.T) (*user.DBUser, error) {
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	return user.GetOrCreateUser(testUser, "Evergreen User", email, accessToken, refreshToken, []string{})
}

func TestRequireProjectSettingsAccess(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()

	pRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		RepoRefId:  "repo_project_id",
	}
	assert.NoError(t, pRef.Insert(t.Context()))

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	apiProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Identifier: utility.ToStringPtr("project_identifier"),
			Admins:     utility.ToStringPtrSlice([]string{"admin_1", "admin_2", "admin_3"}),
		},
	}

	fieldCtx := &graphql.FieldContext{
		Field: graphql.CollectedField{
			Field: &ast.Field{
				Alias: "admins",
			},
		},
	}
	ctx = graphql.WithFieldContext(ctx, fieldCtx)

	res, err := config.Directives.RequireProjectSettingsAccess(ctx, any(nil), next)
	assert.EqualError(t, err, "input: project not valid")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	res, err = config.Directives.RequireProjectSettingsAccess(ctx, apiProjectSettings, next)
	assert.EqualError(t, err, "input: project not specified")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	validApiProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Id:         utility.ToStringPtr("project_id"),
			RepoRefId:  utility.ToStringPtr("repo_project_id"),
			Identifier: utility.ToStringPtr("project_identifier"),
			Admins:     utility.ToStringPtrSlice([]string{"admin_1", "admin_2", "admin_3"}),
		},
	}
	validRepoProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Id: utility.ToStringPtr("repo_project_id"),
		},
	}
	res, err = config.Directives.RequireProjectSettingsAccess(ctx, validApiProjectSettings, next)
	assert.EqualError(t, err, "input: user does not have permission to access the field 'admins' for project with ID 'project_id'")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	// Verify that the user also doesn't have permission to view the repo project
	res, err = config.Directives.RequireProjectSettingsAccess(ctx, validRepoProjectSettings, next)
	assert.EqualError(t, err, "input: user does not have permission to access the field 'admins' for project with ID 'repo_project_id'")
	assert.Nil(t, res)
	assert.Equal(t, 0, callCount)

	err = usr.AddRole(t.Context(), "view_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectSettingsAccess(ctx, validApiProjectSettings, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	// Verify that the user also has permission to view the repo project
	res, err = config.Directives.RequireProjectSettingsAccess(ctx, validRepoProjectSettings, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 2, callCount)
}

func TestRequirePatchOwner(t *testing.T) {
	defer func() {
		require.NoError(t, db.ClearCollections(model.ProjectRefCollection, patch.Collection, user.Collection),
			"unable to clear projectRef, patch, or user collection")

	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId){
		"SucceedsWhenUserIsPatchAuthor": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchIds": []any{patch1Id.Hex()}}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
		},
		"SucceeedsWhenUserIsProjectAdmin": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			assert.NoError(t, usr.AddRole(ctx, "admin_project"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchIds": []any{patch2Id.Hex(), patch3Id.Hex()}}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole(ctx, "admin_project"))
		},
		"SucceedsWhenUserIsSuperUser": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			assert.NoError(t, usr.AddRole(ctx, "superuser"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchIds": []any{patch1Id.Hex(), patch2Id.Hex(), patch3Id.Hex()}}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole(ctx, "superuser"))
		},
		"SucceedsWhenUserIsPatchAdmin": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			assert.NoError(t, usr.AddRole(ctx, "admin_patch"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchIds": []any{patch1Id.Hex(), patch2Id.Hex(), patch3Id.Hex()}}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole(ctx, "admin_patch"))
		},
		"FailsWhenUserIsNotASuperuserOrAdminOrAuthor": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchIds": []any{patch1Id.Hex(), patch2Id.Hex(), patch3Id.Hex()}}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.EqualError(t, err, "input: user 'test_user' does not have permission to modify patches: '64c13ab08edf48a008793cac, 67e2c49e4ebfe83f00ee5f65'")
			assert.Nil(t, res)
			assert.Equal(t, false, nextCalled)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			setupPermissions(t)
			usr, err := setupUser(t)
			assert.NoError(t, err)
			assert.NotNil(t, usr)
			ctx = gimlet.AttachUser(ctx, usr)
			assert.NotNil(t, ctx)
			projectRef := model.ProjectRef{
				Id:         "project_id",
				Identifier: "project_identifier",
			}
			require.NoError(t, projectRef.Insert(t.Context()))
			patch1Id := bson.ObjectIdHex("67e2c29f4ebfe834bb02a482")
			p1 := patch.Patch{
				Id:      patch1Id,
				Project: "project_id",
				Author:  "test_user",
			}
			assert.NoError(t, p1.Insert(t.Context()))
			patch2Id := bson.ObjectIdHex("67e2c49e4ebfe83f00ee5f65")
			p2 := patch.Patch{
				Id:      patch2Id,
				Project: "project_id",
				Author:  "not_test_user",
			}
			assert.NoError(t, p2.Insert(t.Context()))
			patch3Id := bson.ObjectIdHex("64c13ab08edf48a008793cac")
			p3 := patch.Patch{
				Id:      patch3Id,
				Project: "project_id",
				Author:  "not_test_user",
			}
			assert.NoError(t, p3.Insert(t.Context()))
			config := New("/graphql")
			assert.NotNil(t, config)
			next := func(rctx context.Context) (any, error) {
				return nil, nil
			}
			tCase(ctx, t, next, config, usr, patch1Id, patch2Id, patch3Id)
		})
	}
}
func TestRequireAdmin(t *testing.T) {
	defer func() {
		err := db.ClearCollections(user.Collection)
		require.NoError(t, err)
	}()

	obj := any(nil)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, config Config, usr *user.DBUser, wrappedNext func(context.Context) (any, error), nextCalled *bool){
		"SucceedsWhenUserIsAdmin": func(ctx context.Context, t *testing.T, config Config, usr *user.DBUser, wrappedNext func(context.Context) (any, error), nextCalled *bool) {
			role := "superuser"
			require.NoError(t, usr.AddRole(ctx, role))
			res, err := config.Directives.RequireAdmin(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.True(t, utility.FromBoolPtr(nextCalled), "next middleware should be called")
		},
		"FailsWhenUserIsNotAdmin": func(ctx context.Context, t *testing.T, config Config, usr *user.DBUser, wrappedNext func(context.Context) (any, error), nextCalled *bool) {
			role := "regularuser"
			require.NoError(t, usr.AddRole(ctx, role))
			res, err := config.Directives.RequireAdmin(ctx, obj, wrappedNext)
			assert.EqualError(t, err, "input: User 'test_user' lacks required admin permissions", "Expected permission error")
			assert.Nil(t, res)
			assert.False(t, utility.FromBoolPtr(nextCalled))
		},
		"FailsWhenUserHasNoRoles": func(ctx context.Context, t *testing.T, config Config, usr *user.DBUser, wrappedNext func(context.Context) (any, error), nextCalled *bool) {
			res, err := config.Directives.RequireAdmin(ctx, obj, wrappedNext)
			assert.EqualError(t, err, "input: User 'test_user' lacks required admin permissions", "Expected permission error")
			assert.Nil(t, res)
			assert.False(t, utility.FromBoolPtr(nextCalled))
		},
	} {
		setupPermissions(t)
		usr, err := setupUser(t)
		require.NoError(t, err)
		require.NotNil(t, usr)

		ctx := t.Context()
		ctx = gimlet.AttachUser(ctx, usr)
		require.NotNil(t, ctx)

		config := New("/graphql")
		require.NotNil(t, config)

		nextCalled := false
		wrappedNext := func(rctx context.Context) (any, error) {
			nextCalled = true
			return nil, nil
		}
		t.Run(tName, func(t *testing.T) {
			tCase(ctx, t, config, usr, wrappedNext, &nextCalled)
		})
	}
}
