package graphql

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireProjectCreate(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
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

	// callCount keeps track of how many times the function is called
	callCount := 0
	next := func(rctx context.Context) (any, error) {
		ctx = rctx // use context from middleware stack in children
		callCount++
		return nil, nil
	}

	usr, err := user.GetOrCreateUser(t.Context(), testUser, "Mohamed Khelif", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	require.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	require.NotNil(t, ctx)

	// superuser should always be successful, no matter the resolver
	err = usr.AddRole(t.Context(), "superuser")
	require.NoError(t, err)

	res, err := config.Directives.RequireProjectCreate(ctx, nil, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	err = usr.RemoveRole(t.Context(), "superuser")
	require.NoError(t, err)

	res, err = config.Directives.RequireProjectCreate(ctx, nil, next)
	assert.EqualError(t, err, "input: user 'test_user' does not have permission to create projects")
	assert.Nil(t, res)
	assert.Equal(t, 1, callCount)

	err = usr.AddRole(t.Context(), "admin_project")
	require.NoError(t, err)
	res, err = config.Directives.RequireProjectCreate(ctx, nil, next)
	assert.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, 2, callCount)
}
