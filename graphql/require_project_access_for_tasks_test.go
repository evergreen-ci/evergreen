package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestRequireProjectAccessForTasksList(t *testing.T) {
	setupPermissions(t)
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(user.Collection, model.ProjectRefCollection, task.Collection),
			"unable to clear collections")
	})
	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)
	ctx := gimlet.AttachUser(t.Context(), usr)

	next := func(context.Context) (any, error) {
		return nil, nil
	}

	p1 := model.ProjectRef{Id: "project_id", Identifier: "project1", Owner: "evergreen-ci", Repo: "evergreen"}
	require.NoError(t, p1.Insert(ctx))

	p2 := model.ProjectRef{Id: "project_id_2", Identifier: "project2", Owner: "evergreen-ci", Repo: "evergreen"}
	require.NoError(t, p2.Insert(ctx))

	t1 := &task.Task{Id: "task1", Project: p1.Id}
	require.NoError(t, t1.Insert(ctx))

	t2 := &task.Task{Id: "task2", Project: p2.Id}
	require.NoError(t, t2.Insert(ctx))

	grantSuperUserAccess := func(t *testing.T) {
		require.NoError(t, usr.AddRole(ctx, evergreen.SuperUserProjectAccessRole))
		t.Cleanup(func() {
			require.NoError(t, usr.RemoveRole(ctx, evergreen.SuperUserProjectAccessRole))
		})
	}

	t.Run("NonexistentTaskShouldReturnNotFound", func(t *testing.T) {
		obj := map[string]any{"taskIds": []any{"nonexistent"}}
		_, err := RequireProjectAccessForTasks(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
		require.Error(t, err)
		assertGqlErrorCode(t, err, ResourceNotFound)
		assert.EqualError(t, err, "input: tasks not found: 'nonexistent'")
	})
	t.Run("NonexistentTaskAmongExistingTasksShouldReturnNotFound", func(t *testing.T) {
		grantSuperUserAccess(t)
		obj := map[string]any{"taskIds": []any{"nonexistent", t1.Id}}
		_, err := RequireProjectAccessForTasks(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
		require.Error(t, err)
		assertGqlErrorCode(t, err, ResourceNotFound)
		assert.EqualError(t, err, "input: tasks not found: 'nonexistent'")
	})
	t.Run("MissingPermissionOnAnyTaskProjectShouldBeForbidden", func(t *testing.T) {
		obj := map[string]any{"taskIds": []any{t1.Id, t2.Id}}
		_, err := RequireProjectAccessForTasks(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
		require.Error(t, err)
		assertGqlErrorCode(t, err, Forbidden)
		assert.EqualError(t, err, "input: user 'test_user' does not have permission to 'edit tasks' for the project 'project_id'")
	})
	t.Run("PermissionOnAllTaskProjectsShouldSucceed", func(t *testing.T) {
		grantSuperUserAccess(t)
		obj := map[string]any{"taskIds": []any{t1.Id, t2.Id}}
		_, err := RequireProjectAccessForTasks(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
		assert.NoError(t, err)
	})
	t.Run("OtherArgsShouldBeIgnored", func(t *testing.T) {
		grantSuperUserAccess(t)
		obj := map[string]any{"versionId": "nonexistent", "taskIds": []any{t1.Id}}
		_, err := RequireProjectAccessForTasks(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
		assert.NoError(t, err)
	})
	t.Run("MissingTaskIDsShouldReturnInputValidationError", func(t *testing.T) {
		_, err := RequireProjectAccessForTasks(ctx, map[string]any{"versionId": "version1"}, next, ProjectPermissionTasks, AccessLevelEdit)
		require.Error(t, err)
		assertGqlErrorCode(t, err, InputValidationError)
		assert.EqualError(t, err, "input: must specify 'taskIds'")
	})
	t.Run("InvalidTaskIDsShouldReturnInputValidationError", func(t *testing.T) {
		_, err := RequireProjectAccessForTasks(ctx, map[string]any{"taskIds": []any{123}}, next, ProjectPermissionTasks, AccessLevelEdit)
		require.Error(t, err)
		assertGqlErrorCode(t, err, InputValidationError)
		assert.EqualError(t, err, "input: taskIds must be strings")
	})
}

func assertGqlErrorCode(t *testing.T, err error, code GqlError) {
	t.Helper()
	var gqlErr *gqlerror.Error
	require.ErrorAs(t, err, &gqlErr)
	assert.Equal(t, code, gqlErr.Extensions["code"])
}
