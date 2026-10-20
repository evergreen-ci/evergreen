package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	config := NewConfig()
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

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", email, accessToken, refreshToken, []string{})
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
