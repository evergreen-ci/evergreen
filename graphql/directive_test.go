package graphql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
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
	require.Len(t, roles, 0)

	superUserRole := gimlet.Role{
		ID:    "superuser",
		Name:  "superuser",
		Scope: "superuser_scope",
		Permissions: map[string]int{
			evergreen.PermissionProjectCreate: evergreen.ProjectCreate.Value,
			evergreen.PermissionDistroCreate:  evergreen.DistroCreate.Value,
			evergreen.PermissionRoleModify:    evergreen.RoleModify.Value,
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
		Resources: []string{"project_id", "repo_id"},
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
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser){
		"FailsWhenHostIdIsNotSpecified": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser) {
			obj := interface{}(nil)
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: host not specified")
		},
		"FailsWhenHostDoesNotExist": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser) {
			obj := interface{}(map[string]interface{}{"hostId": "a-non-existent-host-id"})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: No matching hosts found")
		},
		"EditFailsWhenUserDoesNotHaveEditPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser) {
			obj := interface{}(map[string]interface{}{"hostId": "host1"})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: user 'testuser' does not have permission to access the host 'host1'")
		},
		"ViewFailsWhenUserDoesNotHaveViewPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser) {
			obj := interface{}(map[string]interface{}{"hostId": "host1"})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelView)
			assert.EqualError(t, err, "input: user 'testuser' does not have permission to access the host 'host1'")
		},
		"EditSucceedsWhenUserHasEditPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser) {
			assert.NoError(t, usr.AddRole("edit_host-id"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (interface{}, error) {
				nextCalled = true
				return nil, nil
			}
			obj := interface{}(map[string]interface{}{"hostId": "host1"})
			res, err := config.Directives.RequireHostAccess(ctx, obj, wrappedNext, HostAccessLevelEdit)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole("edit_host-id"))
		},
		"ViewSucceedsWhenUserHasViewPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (interface{}, error), config Config, usr *user.DBUser) {
			assert.NoError(t, usr.AddRole("view_host-id"))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (interface{}, error) {
				nextCalled = true
				return nil, nil
			}
			obj := interface{}(map[string]interface{}{"hostId": "host1"})
			res, err := config.Directives.RequireHostAccess(ctx, obj, wrappedNext, HostAccessLevelView)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			assert.NoError(t, usr.RemoveRole("view_host-id"))
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
				Id:        "host1",
				StartedBy: "testuser",
				Distro: distro.Distro{
					Id: "distro-id",
				},
			}
			assert.NoError(t, h1.Insert(ctx))
			config := New("/graphql")
			assert.NotNil(t, config)
			next := func(rctx context.Context) (interface{}, error) {
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
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := interface{}(nil)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := user.GetOrCreateUser(apiUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	// Fails when distro is not specified
	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	require.EqualError(t, err, "input: distro not specified")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessCreate)
	require.EqualError(t, err, "input: user 'testuser' does not have create distro permissions")

	// superuser should be successful for create with no distro ID specified
	require.NoError(t, usr.AddRole("superuser"))

	res, err := config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessCreate)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	require.NoError(t, usr.RemoveRole("superuser"))

	// superuser_distro_access is successful for admin, edit, view
	require.NoError(t, usr.AddRole("superuser_distro_access"))

	obj = interface{}(map[string]interface{}{"distroId": "distro-id"})
	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)

	require.NoError(t, usr.RemoveRole("superuser_distro_access"))

	// admin access is successful for admin, edit, view
	require.NoError(t, usr.AddRole("admin_distro-id"))

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 6, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 7, callCount)

	require.NoError(t, usr.RemoveRole("admin_distro-id"))

	// edit access fails for admin, is successful for edit & view
	require.NoError(t, usr.AddRole("edit_distro-id"))

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access settings for the distro 'distro-id'")
	require.Equal(t, 7, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 8, callCount)

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 9, callCount)

	require.NoError(t, usr.RemoveRole("edit_distro-id"))

	// view access fails for admin & edit, is successful for view
	require.NoError(t, usr.AddRole("view_distro-id"))

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access settings for the distro 'distro-id'")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access settings for the distro 'distro-id'")

	res, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 10, callCount)

	require.NoError(t, usr.RemoveRole("view_distro-id"))

	// no access fails all query attempts
	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessAdmin)
	require.Equal(t, 10, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access settings for the distro 'distro-id'")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessEdit)
	require.Equal(t, 10, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access settings for the distro 'distro-id'")

	_, err = config.Directives.RequireDistroAccess(ctx, obj, next, DistroSettingsAccessView)
	require.Equal(t, 10, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access settings for the distro 'distro-id'")
}

func TestRequireProjectAdmin(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()
	obj := interface{}(nil)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := user.GetOrCreateUser(apiUser, "Mohamed Khelif", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}
	err = projectRef.Insert()
	require.NoError(t, err)

	// superuser should always be successful, no matter the resolver
	err = usr.AddRole("superuser")
	require.NoError(t, err)

	res, err := config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	err = usr.RemoveRole("superuser")
	require.NoError(t, err)

	// CreateProject - permission denied
	operationContext := &graphql.OperationContext{
		OperationName: CreateProjectMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]interface{}{
		"project": map[string]interface{}{
			"identifier": "anything",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.EqualError(t, err, "input: user testuser does not have permission to access the CreateProject resolver")
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	// CreateProject - successful
	err = usr.AddRole("admin_project")
	require.NoError(t, err)
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	// CopyProject - permission denied
	operationContext = &graphql.OperationContext{
		OperationName: CopyProjectMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]interface{}{
		"project": map[string]interface{}{
			"projectIdToCopy": "anything",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.EqualError(t, err, "input: user testuser does not have permission to access the CopyProject resolver")
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	// CopyProject - successful
	obj = map[string]interface{}{
		"project": map[string]interface{}{
			"projectIdToCopy": "project_id",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	// DeleteProject - permission denied
	operationContext = &graphql.OperationContext{
		OperationName: DeleteProjectMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]interface{}{"projectId": "anything"}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.EqualError(t, err, "input: user testuser does not have permission to access the DeleteProject resolver")
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	// DeleteProject - successful
	obj = map[string]interface{}{"projectId": "project_id"}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)

	// SetLastRevision - successful
	operationContext = &graphql.OperationContext{
		OperationName: SetLastRevisionMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]interface{}{
		"opts": map[string]interface{}{
			"projectIdentifier": "project_identifier",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)

	// SetLastRevision - project not found
	operationContext = &graphql.OperationContext{
		OperationName: SetLastRevisionMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]interface{}{
		"opts": map[string]interface{}{
			"projectIdentifier": "project_whatever",
		},
	}
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.EqualError(t, err, "input: project 'project_whatever' not found")
	require.Nil(t, res)
	require.Equal(t, 5, callCount)

	// SetLastRevision - permission denied
	operationContext = &graphql.OperationContext{
		OperationName: SetLastRevisionMutation,
	}
	ctx = graphql.WithOperationContext(ctx, operationContext)
	obj = map[string]interface{}{
		"opts": map[string]interface{}{
			"projectIdentifier": "project_identifier",
		},
	}
	require.NoError(t, usr.RemoveRole("admin_project"))
	res, err = config.Directives.RequireProjectAdmin(ctx, obj, next)
	require.EqualError(t, err, "input: user testuser does not have permission to access the SetLastRevision resolver")
	require.Nil(t, res)
	require.Equal(t, 5, callCount)

}

func setupUser(t *testing.T) (*user.DBUser, error) {
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	return user.GetOrCreateUser(apiUser, "Evergreen User", email, accessToken, refreshToken, []string{})
}

func TestRequireProjectSettingsAccess(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
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

	res, err := config.Directives.RequireProjectSettingsAccess(ctx, interface{}(nil), next)
	require.EqualError(t, err, "input: project not valid")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireProjectSettingsAccess(ctx, apiProjectSettings, next)
	require.EqualError(t, err, "input: project not specified")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	validApiProjectSettings := &restModel.APIProjectSettings{
		ProjectRef: restModel.APIProjectRef{
			Id:         utility.ToStringPtr("project_id"),
			Identifier: utility.ToStringPtr("project_identifier"),
			Admins:     utility.ToStringPtrSlice([]string{"admin_1", "admin_2", "admin_3"}),
		},
	}
	res, err = config.Directives.RequireProjectSettingsAccess(ctx, validApiProjectSettings, next)
	require.EqualError(t, err, "input: user does not have permission to access the field 'admins' for project with ID 'project_id'")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	err = usr.AddRole("view_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectSettingsAccess(ctx, validApiProjectSettings, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)
}

func TestRequireCommitQueueItemOwner(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)
	ctx := context.Background()

	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.RepoRefCollection, patch.Collection, commitqueue.Collection))

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	projectRef := model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, projectRef.Insert())

	patch := patch.Patch{
		Id:     bson.NewObjectId(),
		Author: usr.Id,
	}
	require.NoError(t, patch.Insert())

	cq := commitqueue.CommitQueue{
		ProjectID: projectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{Issue: patch.Id.Hex()},
		},
	}
	require.NoError(t, commitqueue.InsertQueue(&cq))

	res, err := config.Directives.RequireCommitQueueItemOwner(ctx, interface{}(nil), next)
	require.EqualError(t, err, "input: converting mutation args into map")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireCommitQueueItemOwner(ctx, map[string]interface{}{}, next)
	require.EqualError(t, err, "input: commit queue id was not provided")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireCommitQueueItemOwner(ctx, map[string]interface{}{
		"commitQueueId": "commit_queue_id",
	}, next)
	require.EqualError(t, err, "input: issue was not provided")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireCommitQueueItemOwner(ctx, map[string]interface{}{
		"commitQueueId": "bad_project",
		"issue":         "123",
	}, next)
	require.EqualError(t, err, "input: 404 (Not Found): project 'bad_project' not found")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireCommitQueueItemOwner(ctx, map[string]interface{}{
		"commitQueueId": projectRef.Id,
		"issue":         patch.Id.Hex(),
	}, next)
	require.EqualError(t, err, "input: 400 (Bad Request): commit queue is not enabled for project 'project_id'")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	projectRef.RepoRefId = "repo_id"
	require.NoError(t, projectRef.Upsert())

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:          projectRef.RepoRefId,
		CommitQueue: model.CommitQueueParams{Enabled: utility.TruePtr()},
	}}
	require.NoError(t, repoRef.Upsert())

	// Should work since the repo and project are merged.
	res, err = config.Directives.RequireCommitQueueItemOwner(ctx, map[string]interface{}{
		"commitQueueId": projectRef.Id,
		"issue":         patch.Id.Hex(),
	}, next)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)
}
