package auth

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	_ "github.com/evergreen-ci/evergreen/testutil" // Import testutil to force init to run
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnlyAPIUserManager(t *testing.T) {
	config := evergreen.AuthConfig{
		AllowServiceUsers: true,
	}
	manager, info, err := LoadUserManager(&evergreen.Settings{AuthConfig: config})
	require.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.Nil(t, manager.GetLoginHandler(""))
	assert.Nil(t, manager.GetLoginCallbackHandler())
	assert.False(t, manager.IsRedirect())

	for testName, testCase := range map[string]func(t *testing.T){
		"DeletesOldOnlyAPIUsers": func(t *testing.T) {
			regUser := &user.DBUser{
				Id:     "reg_user",
				APIKey: "reg_key",
			}
			require.NoError(t, regUser.Insert())
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			assert.NotNil(t, um)

			checkRegUser, err := user.FindOneById(regUser.Id)
			require.NoError(t, err)
			require.NotNil(t, checkRegUser)
			assert.Equal(t, regUser.APIKey, checkRegUser.APIKey)

			checkAPIUser, err := user.FindOneById(apiUser.Id)
			assert.NoError(t, err)
			assert.Nil(t, checkAPIUser)
		},
		"GetUserByIDSucceeds": func(t *testing.T) {
			u1 := user.DBUser{
				Id:      "user1",
				APIKey:  "key1",
				OnlyAPI: true,
			}
			assert.NoError(t, u1.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			checkUser, err := um.GetUserByID(u1.Id)
			require.NoError(t, err)
			checkDBUser, ok := checkUser.(*user.DBUser)
			require.True(t, ok, "user manager in evergreen must return DBUser")
			assert.Equal(t, u1.APIKey, checkDBUser.APIKey)
			assert.True(t, checkDBUser.OnlyAPI)
		},
		"GetUserByIDFailsIfNonexistent": func(t *testing.T) {
			u := user.DBUser{
				Id:      "user1",
				APIKey:  "key1",
				OnlyAPI: true,
			}
			assert.NoError(t, u.Insert())

			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			checkUser, err := um.GetUserByID("nonexistent")
			assert.Error(t, err)
			assert.Nil(t, checkUser)
		},
		"GetUserByIDFailsIfNotOnlyAPIUser": func(t *testing.T) {
			regUser := &user.DBUser{
				Id:     "reg_user",
				APIKey: "reg_key",
			}
			require.NoError(t, regUser.Insert())
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			checkUser, err := um.GetUserByID(regUser.Id)
			assert.Error(t, err)
			assert.Nil(t, checkUser)
		},
		"GetUserByTokenFails": func(t *testing.T) {
			apiUser := &user.DBUser{
				Id: "api_user",
				LoginCache: user.LoginCache{
					Token: "token",
				},
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			checkUser, err := um.GetUserByToken(context.Background(), apiUser.LoginCache.Token)
			assert.Error(t, err)
			assert.Nil(t, checkUser)
		},
		"CreateUserTokenFails": func(t *testing.T) {
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			token, err := um.CreateUserToken(apiUser.Id, "")
			assert.Error(t, err)
			assert.Empty(t, token)

			token, err = um.CreateUserToken("new_api_user", "")
			assert.Error(t, err)
			assert.Empty(t, token)
		},
		"ClearUserFails": func(t *testing.T) {
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			assert.Error(t, um.ClearUser(apiUser, false))
			checkUser, err := user.FindOneById(apiUser.Id)
			require.NoError(t, err)
			require.NotNil(t, checkUser)
			assert.Equal(t, checkUser.APIKey, apiUser.APIKey)

			assert.Error(t, um.ClearUser(apiUser, true))
			checkUser, err = user.FindOneById(apiUser.Id)
			require.NoError(t, err)
			require.NotNil(t, checkUser)
			assert.Equal(t, checkUser.APIKey, apiUser.APIKey)

			assert.Error(t, um.ClearUser(&user.DBUser{}, true))
			checkUser, err = user.FindOneById(apiUser.Id)
			require.NoError(t, err)
			require.NotNil(t, checkUser)
			assert.Equal(t, checkUser.APIKey, apiUser.APIKey)
		},
		"ReauthorizeUserFails": func(t *testing.T) {
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			regUser := &user.DBUser{
				Id:     "reg_user",
				APIKey: "reg_key",
			}
			require.NoError(t, regUser.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			assert.Error(t, um.ReauthorizeUser(apiUser))
			assert.Error(t, um.ReauthorizeUser(regUser))
		},
		"GetGroupsForUserFails": func(t *testing.T) {
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			regUser := &user.DBUser{
				Id:     "reg_user",
				APIKey: "reg_key",
			}
			require.NoError(t, regUser.Insert())
			um, err := NewOnlyAPIUserManager()
			require.NoError(t, err)
			require.NotNil(t, um)

			groups, err := um.GetGroupsForUser(apiUser.Id)
			assert.Error(t, err)
			assert.Empty(t, groups)
			groups, err = um.GetGroupsForUser(regUser.Id)
			assert.Error(t, err)
			assert.Empty(t, groups)
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
