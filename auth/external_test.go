package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalUserManager(t *testing.T) {
	require.NoError(t, db.Clear(user.Collection))
	defer func() { require.NoError(t, db.Clear(user.Collection)) }()

	userID := "bobby.tables"
	displayName := "Bobby Tables"
	usr, err := user.GetOrCreateUser(t.Context(), userID, displayName, "", "", "", nil)
	require.NoError(t, err)

	mgr, err := NewExternalUserManager()
	assert.NoError(t, err)

	t.Run("GetUserByID", func(t *testing.T) {
		mgrUsr, err := mgr.GetUserByID(t.Context(), userID)
		assert.NoError(t, err)
		assert.Equal(t, usr, mgrUsr)
	})

	t.Run("GetOrCreateUser", func(t *testing.T) {
		mgrUsr, err := mgr.GetOrCreateUser(t.Context(), usr)
		assert.NoError(t, err)
		assert.Equal(t, usr, mgrUsr)
	})

	t.Run("GetUserByToken", func(t *testing.T) {
		mgrUsr, err := mgr.GetUserByToken(t.Context(), "abc")
		assert.Error(t, err)
		assert.Nil(t, mgrUsr)
	})

	t.Run("ReauthorizeUser", func(t *testing.T) {
		assert.Error(t, mgr.ReauthorizeUser(t.Context(), usr))
	})

	t.Run("CreateUserToken", func(t *testing.T) {
		token, err := mgr.CreateUserToken(t.Context(), usr.Id, "abc")
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
		assert.Error(t, mgr.ClearUser(t.Context(), usr, false))
	})
}
