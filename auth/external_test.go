package auth

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalUserManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(user.Collection))
	defer func() { require.NoError(t, db.Clear(user.Collection)) }()

	userID := "bobby.tables"
	displayName := "Bobby Tables"
	usr, err := user.GetOrCreateUser(userID, displayName, "", "", "", nil)
	require.NoError(t, err)

	mgr, err := NewExternalUserManager()
	assert.NoError(t, err)

	t.Run("GetUserByID", func(t *testing.T) {
		mgrUsr, err := mgr.GetUserByID(userID)
		assert.NoError(t, err)
		assert.Equal(t, usr, mgrUsr)
	})

	t.Run("GetOrCreateUser", func(t *testing.T) {
		mgrUsr, err := mgr.GetOrCreateUser(usr)
		assert.NoError(t, err)
		assert.Equal(t, usr, mgrUsr)
	})

	t.Run("GetUserByToken", func(t *testing.T) {
		mgrUsr, err := mgr.GetUserByToken(ctx, "abc")
		assert.Error(t, err)
		assert.Nil(t, mgrUsr)
	})

	t.Run("ReauthorizeUser", func(t *testing.T) {
		assert.Error(t, mgr.ReauthorizeUser(usr))
	})

	t.Run("CreateUserToken", func(t *testing.T) {
		token, err := mgr.CreateUserToken(usr.Id, "abc")
		assert.Error(t, err)
		assert.Empty(t, token)
	})

	t.Run("GetLoginHandler", func(t *testing.T) {
		assert.Nil(t, mgr.GetLoginHandler("abc"))
	})

	t.Run("GetLoginCallbackHandler", func(t *testing.T) {
		assert.Nil(t, mgr.GetLoginCallbackHandler())
	})

	t.Run("IsRedirect", func(t *testing.T) {
		assert.False(t, mgr.IsRedirect())
	})

	t.Run("ClearUser", func(t *testing.T) {
		assert.Error(t, mgr.ClearUser(usr, false))
	})
}
