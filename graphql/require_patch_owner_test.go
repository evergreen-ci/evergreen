package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		"SucceedsWithPatchIdWhenUserIsAuthor": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchId": patch1Id.Hex()}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.True(t, nextCalled)
		},
		"FailsWithPatchIdWhenUserIsNotOwner": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser, patch1Id bson.ObjectId, patch2Id bson.ObjectId, patch3Id bson.ObjectId) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := map[string]any{"patchId": patch2Id.Hex()}
			res, err := config.Directives.RequirePatchOwner(ctx, obj, wrappedNext)
			assert.Error(t, err)
			assert.Nil(t, res)
			assert.False(t, nextCalled)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()
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
			config := NewConfig()
			assert.NotNil(t, config)
			next := func(rctx context.Context) (any, error) {
				return nil, nil
			}
			tCase(ctx, t, next, config, usr, patch1Id, patch2Id, patch3Id)
		})
	}
}
