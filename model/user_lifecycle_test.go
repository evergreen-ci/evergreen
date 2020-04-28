package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrCreateUser(t *testing.T) {
	id := "id"
	name := "name"
	email := "email"
	accessToken := "access_token"
	refreshToken := "refresh_token"

	checkUser := func(t *testing.T, user *user.DBUser, id, name, email, accessToken, refreshToken string) {
		assert.Equal(t, id, user.Username())
		assert.Equal(t, name, user.DisplayName())
		assert.Equal(t, email, user.Email())
		assert.Equal(t, accessToken, user.GetAccessToken())
		assert.Equal(t, refreshToken, user.GetRefreshToken())
	}

	for testName, testCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			user, err := GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			apiKey := user.GetAPIKey()
			assert.NotEmpty(t, apiKey)

			dbUser, err := FindUserByID(id)
			require.NoError(t, err)
			checkUser(t, dbUser, id, name, email, accessToken, refreshToken)
			assert.Equal(t, apiKey, dbUser.GetAPIKey())
		},
		"UpdateAlwaysSetsNameAndEmail": func(t *testing.T) {
			user, err := GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			apiKey := user.GetAPIKey()
			assert.NotEmpty(t, apiKey)

			newName := "new_name"
			newEmail := "new_email"
			user, err = GetOrCreateUser(id, newName, newEmail, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, newName, newEmail, accessToken, refreshToken)
			assert.Equal(t, apiKey, user.GetAPIKey())
		},
		"UpdateSetsTokensIfNonempty": func(t *testing.T) {
			user, err := GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			apiKey := user.GetAPIKey()
			assert.NotEmpty(t, apiKey)

			user, err = GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			assert.Equal(t, apiKey, user.GetAPIKey())

			user, err = GetOrCreateUser(id, name, email, "", "", nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			assert.Equal(t, apiKey, user.GetAPIKey())

			newAccessToken := "new_access_token"
			newRefreshToken := "new_refresh_token"
			user, err = GetOrCreateUser(id, name, email, newAccessToken, newRefreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, newAccessToken, newRefreshToken)
			assert.Equal(t, apiKey, user.GetAPIKey())
		},
		"RolesMergeCorrectly": func(t *testing.T) {
			roles := []string{"one", "two"}
			user, err := GetOrCreateUser(id, "", "", "", "", roles)
			assert.NoError(t, err)
			checkUser(t, user, id, "id", "", "", "")
			assert.Equal(t, roles, user.Roles())

			user, err = GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			assert.NoError(t, err)
			checkUser(t, user, id, name, email, accessToken, refreshToken)
			assert.Equal(t, roles, user.Roles())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()
			testCase(t)
		})
	}
}
