package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireVolumeAccess(t *testing.T) {
	defer func() {
		require.NoError(t, db.ClearCollections(host.VolumesCollection, host.Collection, user.Collection),
			"unable to clear user, volumes, or host collection")
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser){
		"FailsWhenInputIsInvalid": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser) {
			obj := any(nil)
			_, err := RequireVolumeAccess(ctx, obj, next)
			assert.EqualError(t, err, "input: converting args into map")
		},
		"FailsWhenVolumeIdIsNotSpecified": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser) {
			obj := any(map[string]any{"volumeId": ""})
			_, err := RequireVolumeAccess(ctx, obj, next)
			assert.EqualError(t, err, "input: must specify volume ID")
		},
		"FailsWhenVolumeDoesNotExist": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser) {
			obj := any(map[string]any{"volumeId": "a-non-existent-volume-id"})
			_, err := RequireVolumeAccess(ctx, obj, next)
			assert.EqualError(t, err, "input: volume 'a-non-existent-volume-id' not found")
		},
		"FailsWhenUserDoesNotHavePermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser) {
			obj := any(map[string]any{"volumeId": "vol2"})
			_, err := RequireVolumeAccess(ctx, obj, next)
			assert.EqualError(t, err, "input: user 'test_user' does not have permission to access volume 'vol2'")
		},
		"SucceedsWhenUserHasPermission": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser) {
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := any(map[string]any{"volumeId": "vol1"})
			res, err := RequireVolumeAccess(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
		},
		"AdminCanBypassPermissionsCheck": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), usr *user.DBUser) {
			role := "superuser"
			require.NoError(t, usr.AddRole(ctx, role))
			nextCalled := false
			wrappedNext := func(rctx context.Context) (any, error) {
				nextCalled = true
				return nil, nil
			}
			obj := any(map[string]any{"volumeId": "vol2"})
			res, err := RequireVolumeAccess(ctx, obj, wrappedNext)
			assert.NoError(t, err)
			assert.Nil(t, res)
			assert.Equal(t, true, nextCalled)
			require.NoError(t, usr.RemoveRole(ctx, role))
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
			v1 := host.Volume{
				ID:        "vol1",
				CreatedBy: usr.Id,
			}
			assert.NoError(t, v1.Insert(ctx))
			v2 := host.Volume{
				ID:        "vol2",
				CreatedBy: "mci_admin",
			}
			assert.NoError(t, v2.Insert(ctx))
			next := func(rctx context.Context) (any, error) {
				return nil, nil
			}
			tCase(ctx, t, next, usr)
		})
	}
}
