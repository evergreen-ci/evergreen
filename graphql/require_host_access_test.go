package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireHostAccess(t *testing.T) {
	defer func() {
		require.NoError(t, db.ClearCollections(host.Collection, user.Collection),
			"unable to clear user or host collection")
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser){
		"FailsWhenInputIsInvalid": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			obj := any(nil)
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: converting args into map")
		},
		"FailsWhenHostIdIsNotSpecified": func(ctx context.Context, t *testing.T, next func(rctx context.Context) (any, error), config Config, usr *user.DBUser) {
			obj := any(map[string]any{"hostId": ""})
			_, err := config.Directives.RequireHostAccess(ctx, obj, next, HostAccessLevelEdit)
			assert.EqualError(t, err, "input: must specify host ID(s)")
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
			ctx := t.Context()
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
			config := NewConfig()
			assert.NotNil(t, config)
			next := func(rctx context.Context) (any, error) {
				return nil, nil
			}
			tCase(ctx, t, next, config, usr)
		})
	}
}
