package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
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
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// error if input is invalid
	obj := any(nil)
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: converting args into map")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	// error if no valid parameters
	obj = any(map[string]any(nil))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: params map is empty")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	// error if invalid permission and access combination
	obj = any(map[string]any(nil))
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
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		RepoRefId:  "repo_id",
	}
	err = projectRef.Insert(t.Context())
	require.NoError(t, err)

	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id: "repo_id",
	}}
	err = repoRef.Replace(ctx)
	require.NoError(t, err)

	obj := any(map[string]any{"projectIdentifier": "invalid_identifier"})
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: project/repo 'invalid_identifier' not found")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj = any(map[string]any{"projectIdentifier": projectRef.Identifier})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit project settings' for the project 'project_id'")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	err = usr.AddRole(t.Context(), "view_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit project settings' for the project 'project_id'")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	err = usr.AddRole(t.Context(), "admin_project")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)

	obj = any(map[string]any{"projectIdentifier": projectRef.Identifier})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	// Verify that user with only branch permission can view the repo page but not edit.
	obj = any(map[string]any{"repoId": repoRef.ProjectRef.Id})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelEdit)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit project settings' for the project 'repo_id'")
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	obj = any(map[string]any{"repoId": repoRef.ProjectRef.Id})
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionSettings, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
}

func TestRequireProjectAccessForTasks(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, task.Collection),
		"unable to clear user & task collections")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert(t.Context()))

	task := &task.Task{
		Id:      "task_id",
		Project: project.Id,
	}
	require.NoError(t, task.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := any(map[string]any{"taskId": task.Id})

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for admin, edit, view
	require.NoError(t, usr.AddRole(t.Context(), "admin_project_access"))
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
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_project_access"))

	// admin access is successful for admin, edit, view
	require.NoError(t, usr.AddRole(t.Context(), "admin_task"))
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
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_task"))

	// edit access fails for admin, is successful for edit & view
	require.NoError(t, usr.AddRole(t.Context(), "edit_task"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit tasks and override dependencies' for the project 'project_id'")
	require.Equal(t, 6, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 7, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 8, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "edit_task"))

	// view access fails for admin & edit, is successful for view
	require.NoError(t, usr.AddRole(t.Context(), "view_task"))
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.Equal(t, 8, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit tasks and override dependencies' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.Equal(t, 8, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit tasks' for the project 'project_id'")

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 9, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "view_task"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelAdmin)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit tasks and override dependencies' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit tasks' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionTasks, AccessLevelView)
	require.Equal(t, 9, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'view tasks' for the project 'project_id'")
}

func TestRequireProjectAccessForAnnotations(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, task.Collection),
		"unable to clear user & task collections")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert(t.Context()))

	task := &task.Task{
		Id:      "task_id",
		Project: project.Id,
	}
	require.NoError(t, task.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := any(map[string]any{"taskId": task.Id})

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for edit, view
	require.NoError(t, usr.AddRole(t.Context(), "admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_project_access"))

	// edit access is successful for edit, view
	require.NoError(t, usr.AddRole(t.Context(), "edit_annotation"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "edit_annotation"))

	// view access fails for edit, is successful for view
	require.NoError(t, usr.AddRole(t.Context(), "view_annotation"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'modify annotations' for the project 'project_id'")
	require.Equal(t, 4, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "view_annotation"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelEdit)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'modify annotations' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionAnnotations, AccessLevelView)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'view annotations' for the project 'project_id'")
}

func TestRequireProjectAccessForPatches(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection, patch.Collection), "unable to clear user & patch collections")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert(t.Context()))

	patch := &patch.Patch{
		Id:      bson.NewObjectId(),
		Project: project.Id,
	}
	require.NoError(t, patch.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := any(map[string]any{"patchId": patch.Id.Hex()})

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for admin, edit
	require.NoError(t, usr.AddRole(t.Context(), "admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_project_access"))

	// admin access is successful for admin, edit
	require.NoError(t, usr.AddRole(t.Context(), "admin_patch"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 3, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 4, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_patch"))

	// edit access fails for admin, is successful for edit
	require.NoError(t, usr.AddRole(t.Context(), "edit_patch"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.Nil(t, res)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'submit/edit patches, and submit patches on behalf of users' for the project 'project_id'")
	require.Equal(t, 4, callCount)

	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 5, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "edit_patch"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelAdmin)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'submit/edit patches, and submit patches on behalf of users' for the project 'project_id'")

	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
	require.Equal(t, 5, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'submit and edit patches' for the project 'project_id'")
}

func TestRequireProjectAccessForLogs(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(user.Collection), "unable to clear user collection")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))

	project := &model.ProjectRef{
		Id: "project_id",
	}
	require.NoError(t, project.Insert(t.Context()))

	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	config := New("/graphql")
	require.NotNil(t, config)
	obj := any(map[string]any{"projectId": project.Id})

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	// superuser should be successful for view
	require.NoError(t, usr.AddRole(t.Context(), "admin_project_access"))
	res, err := config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionLogs, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "admin_project_access"))

	// view access is successful for view
	require.NoError(t, usr.AddRole(t.Context(), "view_logs"))
	res, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionLogs, AccessLevelView)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
	require.NoError(t, usr.RemoveRole(t.Context(), "view_logs"))

	// no access fails all query attempts
	_, err = config.Directives.RequireProjectAccess(ctx, obj, next, ProjectPermissionLogs, AccessLevelView)
	require.Equal(t, 2, callCount)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'view logs' for the project 'project_id'")
}

func TestRequireRepoAccess(t *testing.T) {
	setupPermissions(t)
	config := New("/graphql")
	require.NotNil(t, config)

	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := gimlet.AttachUser(context.Background(), usr)
	require.NotNil(t, ctx)

	// The project is not yet attached to a repo; its owner/repo determine which
	// repo ref it would attach to.
	projectRef := model.ProjectRef{
		Id:         "project_id",
		Identifier: "project_identifier",
		Owner:      "evergreen-ci",
		Repo:       "spruce",
	}
	require.NoError(t, projectRef.Insert(t.Context()))

	callCount := 0
	next := func(rctx context.Context) (any, error) {
		callCount++
		return nil, nil
	}

	res, err := config.Directives.RequireRepoAccess(ctx, any(nil), next, AccessLevelAdmin)
	require.EqualError(t, err, "input: converting args into map")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireRepoAccess(ctx, any(map[string]any{}), next, AccessLevelAdmin)
	require.EqualError(t, err, "input: project not specified")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	res, err = config.Directives.RequireRepoAccess(ctx, any(map[string]any{"projectId": "nonexistent"}), next, AccessLevelAdmin)
	require.EqualError(t, err, "input: project 'nonexistent' not found")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	obj := any(map[string]any{"projectId": projectRef.Id})

	// No repo ref exists yet and the user is not a project admin, so they may
	// not attach.
	res, err = config.Directives.RequireRepoAccess(ctx, obj, next, AccessLevelAdmin)
	require.EqualError(t, err, "input: user 'test_user' must be an admin of project 'project_id' to attach it to a new repo")
	require.Nil(t, res)
	require.Equal(t, 0, callCount)

	// A project admin may attach the project to a not-yet-existing repo.
	require.NoError(t, usr.AddRole(t.Context(), "admin_project"))
	res, err = config.Directives.RequireRepoAccess(ctx, obj, next, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	// Once a repo ref exists, ADMIN access requires repo admin even for a
	// project admin.
	repoRef := model.RepoRef{ProjectRef: model.ProjectRef{
		Id:    "repo_id",
		Owner: "evergreen-ci",
		Repo:  "spruce",
	}}
	require.NoError(t, repoRef.Replace(t.Context()))

	res, err = config.Directives.RequireRepoAccess(ctx, obj, next, AccessLevelAdmin)
	require.EqualError(t, err, "input: user 'test_user' is not an admin of repo 'evergreen-ci/spruce'")
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	// An unsupported access level is rejected.
	res, err = config.Directives.RequireRepoAccess(ctx, obj, next, AccessLevelEdit)
	require.EqualError(t, err, "input: invalid access level 'EDIT' for repo")
	require.Nil(t, res)
	require.Equal(t, 1, callCount)

	// Grant the user repo edit access, making them a repo admin.
	roleManager := evergreen.GetEnvironment().RoleManager()
	repoScope := gimlet.Scope{
		ID:        "repo_scope",
		Name:      "repo scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{repoRef.Id},
	}
	require.NoError(t, roleManager.AddScope(t.Context(), repoScope))
	repoAdminRole := gimlet.Role{
		ID:    "admin_repo",
		Scope: repoScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		},
	}
	require.NoError(t, roleManager.UpdateRole(t.Context(), repoAdminRole))
	require.NoError(t, usr.AddRole(t.Context(), "admin_repo"))

	res, err = config.Directives.RequireRepoAccess(ctx, obj, next, AccessLevelAdmin)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Equal(t, 2, callCount)
}
