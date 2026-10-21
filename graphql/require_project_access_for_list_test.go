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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestRequireProjectAccessForList(t *testing.T) {
	defer func() {
		require.NoError(t, db.ClearCollections(user.Collection, model.ProjectRefCollection, model.VersionCollection, task.Collection, patch.Collection),
			"unable to clear collections")
	}()

	config := NewConfig()
	require.NotNil(t, config)

	setupPermissions(t)
	usr, err := setupUser(t)
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx := t.Context()
	ctx = gimlet.AttachUser(ctx, usr)

	next := func(rctx context.Context) (any, error) {
		return nil, nil
	}

	project1 := &model.ProjectRef{
		Id:         "project_id",
		Identifier: "project1",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
	}
	require.NoError(t, project1.Insert(ctx))

	project2 := &model.ProjectRef{
		Id:         "project_id_2",
		Identifier: "project2",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
	}
	require.NoError(t, project2.Insert(ctx))

	t.Run("VersionIds", func(t *testing.T) {
		version1 := &model.Version{
			Id:         "version1",
			Identifier: project1.Id,
		}
		require.NoError(t, version1.Insert(ctx))

		version2 := &model.Version{
			Id:         "version2",
			Identifier: project2.Id,
		}
		require.NoError(t, version2.Insert(ctx))

		t.Run("FailsWhenDoesNotExist", func(t *testing.T) {
			obj := map[string]any{"versionIds": []any{"nonexistent"}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
			require.Error(t, err)
			assertGqlErrorCode(t, err, ResourceNotFound)
		})
		t.Run("FailsWithoutPermission", func(t *testing.T) {
			obj := map[string]any{"versionIds": []any{"version1", "version2"}}
			_, err := config.Directives.RequireProjectAccessForList(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
			require.Error(t, err)
			assertGqlErrorCode(t, err, Forbidden)
		})
		t.Run("SucceedsWithPermission", func(t *testing.T) {
			require.NoError(t, usr.AddRole(ctx, evergreen.SuperUserProjectAccessRole))
			defer func() {
				require.NoError(t, usr.RemoveRole(ctx, evergreen.SuperUserProjectAccessRole))
			}()
			obj := map[string]any{"versionIds": []any{"version1", "version2"}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
			assert.NoError(t, err)
		})
	})

	t.Run("TaskIds", func(t *testing.T) {
		task1 := &task.Task{
			Id:      "task1",
			Project: project1.Id,
		}
		require.NoError(t, task1.Insert(ctx))
		task2 := &task.Task{
			Id:      "task2",
			Project: project2.Id,
		}
		require.NoError(t, task2.Insert(ctx))

		t.Run("FailsWhenDoesNotExist", func(t *testing.T) {
			obj := map[string]any{"taskIds": []any{"nonexistent"}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
			require.Error(t, err)
			assertGqlErrorCode(t, err, ResourceNotFound)
		})
		t.Run("FailsWithoutPermission", func(t *testing.T) {
			obj := map[string]any{"taskIds": []any{"task1", "task2"}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
			require.Error(t, err)
			assertGqlErrorCode(t, err, Forbidden)
		})
		t.Run("SucceedsWithPermission", func(t *testing.T) {
			require.NoError(t, usr.AddRole(ctx, evergreen.SuperUserProjectAccessRole))
			defer func() {
				require.NoError(t, usr.RemoveRole(ctx, evergreen.SuperUserProjectAccessRole))
			}()
			obj := map[string]any{"taskIds": []any{"task1", "task2"}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionTasks, AccessLevelEdit)
			assert.NoError(t, err)
		})
	})

	t.Run("PatchIds", func(t *testing.T) {
		patch1 := &patch.Patch{
			Id:      bson.NewObjectId(),
			Project: project1.Id,
		}
		require.NoError(t, patch1.Insert(ctx))

		patch2 := &patch.Patch{
			Id:      bson.NewObjectId(),
			Project: project2.Id,
		}
		require.NoError(t, patch2.Insert(ctx))

		t.Run("FailsWhenInvalid", func(t *testing.T) {
			obj := map[string]any{"patchIds": []any{"not_a_valid_id"}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
			require.Error(t, err)
			assertGqlErrorCode(t, err, InputValidationError)
		})
		t.Run("FailsWithoutPermission", func(t *testing.T) {
			obj := map[string]any{"patchIds": []any{patch1.Id.Hex(), patch2.Id.Hex()}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
			require.Error(t, err)
			assertGqlErrorCode(t, err, Forbidden)
		})
		t.Run("SucceedsWithPermission", func(t *testing.T) {
			require.NoError(t, usr.AddRole(ctx, evergreen.SuperUserProjectAccessRole))
			defer func() {
				require.NoError(t, usr.RemoveRole(ctx, evergreen.SuperUserProjectAccessRole))
			}()
			obj := map[string]any{"patchIds": []any{patch1.Id.Hex(), patch2.Id.Hex()}}
			_, err := RequireProjectAccessForList(ctx, obj, next, ProjectPermissionPatches, AccessLevelEdit)
			assert.NoError(t, err)
		})
	})

	t.Run("NoListArgFailsWithInputValidationError", func(t *testing.T) {
		_, err := RequireProjectAccessForList(ctx, map[string]any{"other": []any{"x"}}, next, ProjectPermissionTasks, AccessLevelEdit)
		require.Error(t, err)
		assertGqlErrorCode(t, err, InputValidationError)
	})
}

func assertGqlErrorCode(t *testing.T, err error, code GqlError) {
	t.Helper()
	var gqlErr *gqlerror.Error
	require.ErrorAs(t, err, &gqlErr)
	assert.Equal(t, code, gqlErr.Extensions["code"])
}
