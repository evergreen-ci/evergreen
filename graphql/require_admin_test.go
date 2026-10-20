package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

		config := NewConfig()
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
