package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/require"
)

func TestRequireProjectAccess(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, task.Collection),
		"unable to clear user & task collections")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)

	usr, err := user.GetOrCreateUser(apiUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// error if input is invalid
	obj := interface{}(nil)
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: converting args into map")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	// error if no valid parameters
	obj = interface{}(map[string]interface{}(nil))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: params map is empty")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	// error if invalid permission and access combination
	obj = interface{}(map[string]interface{}(nil))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelAdmin)
	require.EqualError(t, err, "input: invalid permission and access level configuration: invalid access level for project_task_annotations")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)
}

func TestRequireProjectAccessForSettings(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
	}
	err = projectRef.Insert()
	require.NoError(t, err)

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id: "repo_id",
	}}
	err = repoRef.Upsert()
	require.NoError(t, err)

	obj := interface{}(map[string]interface{}{"projectIdentifier": "invalid_identifier"})
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: project 'invalid_identifier' not found")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj = interface{}(map[string]interface{}{"projectIdentifier": projectRef.Identifier})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'settings' for the project 'project_id'")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	err = usr.AddRole("view_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'settings' for the project 'project_id'")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	err = usr.AddRole("admin_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	obj = interface{}(map[string]interface{}{"projectIdentifier": projectRef.Identifier})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	obj = interface{}(map[string]interface{}{"repoId": repoRef.ProjectRef.Id})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
}

func TestRequireProjectAccessForTasks(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, task.Collection),
		"unable to clear user & task collections")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert())

	task := &task.Task{
		Id:      "task_id",
		Project: project.Id,
	}
	require.NoError(t, task.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := interface{}(map[string]interface{}{"taskId": task.Id})

	usr, err := user.GetOrCreateUser(apiUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for admin, edit, view
	require.NoError(t, usr.AddRole("admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)
	require.NoError(t, usr.RemoveRole("admin_project_access"))

	// admin access is successful for admin, edit, view
	require.NoError(t, usr.AddRole("admin_task"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 6, callCount)
	require.NoError(t, usr.RemoveRole("admin_task"))

	// edit access fails for admin, is successful for edit & view
	require.NoError(t, usr.AddRole("edit_task"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'tasks' for the project 'project_id'")
	require.Equal(t, 6, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 7, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 8, callCount)
	require.NoError(t, usr.RemoveRole("edit_task"))

	// view access fails for admin & edit, is successful for view
	require.NoError(t, usr.AddRole("view_task"))
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.Equal(t, 8, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'tasks' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.Equal(t, 8, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'tasks' for the project 'project_id'")

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 9, callCount)
	require.NoError(t, usr.RemoveRole("view_task"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'tasks' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'tasks' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'tasks' for the project 'project_id'")
}

func TestRequireProjectAccessForAnnotations(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, task.Collection),
		"unable to clear user & task collections")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert())

	task := &task.Task{
		Id:      "task_id",
		Project: project.Id,
	}
	require.NoError(t, task.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := interface{}(map[string]interface{}{"taskId": task.Id})

	usr, err := user.GetOrCreateUser(apiUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for edit, view
	require.NoError(t, usr.AddRole("admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
	require.NoError(t, usr.RemoveRole("admin_project_access"))

	// edit access is successful for edit, view
	require.NoError(t, usr.AddRole("edit_annotation"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
	require.NoError(t, usr.RemoveRole("edit_annotation"))

	// view access fails for edit, is successful for view
	require.NoError(t, usr.AddRole("view_annotation"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'annotations' for the project 'project_id'")
	require.Equal(t, 4, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)
	require.NoError(t, usr.RemoveRole("view_annotation"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'annotations' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'annotations' for the project 'project_id'")
}

func TestRequireProjectAccessForPatches(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, patch.Collection), "unable to clear user & patch collections")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert())

	patch := &patch.Patch{
		Id:      bson.NewObjectId(),
		Project: project.Id,
	}
	require.NoError(t, patch.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := interface{}(map[string]interface{}{"patchId": patch.Id.Hex()})

	usr, err := user.GetOrCreateUser(apiUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for admin, edit
	require.NoError(t, usr.AddRole("admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
	require.NoError(t, usr.RemoveRole("admin_project_access"))

	// admin access is successful for admin, edit
	require.NoError(t, usr.AddRole("admin_patch"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
	require.NoError(t, usr.RemoveRole("admin_patch"))

	// edit access fails for admin, is successful for edit
	require.NoError(t, usr.AddRole("edit_patch"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'patches' for the project 'project_id'")
	require.Equal(t, 4, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)
	require.NoError(t, usr.RemoveRole("edit_patch"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'patches' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'patches' for the project 'project_id'")
}

func TestRequireProjectAccessForLogs(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection), "unable to clear user collection")
	dbUser := &user.DBUser{
		Id: apiUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert())

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert())

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := interface{}(map[string]interface{}{"projectId": project.Id})

	usr, err := user.GetOrCreateUser(apiUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (interface{}, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for view
	require.NoError(t, usr.AddRole("admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionLogs, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)
	require.NoError(t, usr.RemoveRole("admin_project_access"))

	// view access is successful for view
	require.NoError(t, usr.AddRole("view_logs"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionLogs, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
	require.NoError(t, usr.RemoveRole("view_logs"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionLogs, AccessLevelView)
	require.Equal(t, 2, callCount)
	require.EqualError(t, err, "input: user 'testuser' does not have permission to access 'logs' for the project 'project_id'")
}
