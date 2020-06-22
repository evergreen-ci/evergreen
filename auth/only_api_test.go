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
		OnlyAPI: &evergreen.OnlyAPIAuthConfig{},
	}
	manager, info, err := LoadUserManager(&evergreen.Settings{AuthConfig: config})
	require.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.Nil(t, manager.GetLoginHandler(""))
	assert.Nil(t, manager.GetLoginCallbackHandler())
	assert.False(t, manager.IsRedirect())

	checkUser := func(t *testing.T, u evergreen.OnlyAPIUser) {
		dbUser, err := user.FindOneById(u.Username)
		require.NoError(t, err)
		require.NotNil(t, dbUser)
		assert.Equal(t, u.Key, dbUser.GetAPIKey())
		assert.Equal(t, u.Key, dbUser.GetAPIKey())
		assert.True(t, dbUser.OnlyAPI)
		assert.Subset(t, u.Roles, dbUser.Roles())
		assert.Subset(t, dbUser.Roles(), u.Roles)
	}

	for testName, testCase := range map[string]func(t *testing.T){
		"InsertsNewUsers": func(t *testing.T) {
			u1 := evergreen.OnlyAPIUser{
				Username: "api_user1",
				Key:      "api_key1",
				Roles:    []string{"role1"},
			}
			u2 := evergreen.OnlyAPIUser{
				Username: "api_user2",
				Key:      "api_key2",
				Roles:    []string{"role2"},
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u1, u2},
			}
			um, err := NewOnlyAPIUserManager(conf)
			require.NoError(t, err)
			assert.NotNil(t, um)

			checkUser(t, u1)
			checkUser(t, u2)
		},
		"UpdatesExistingUsers": func(t *testing.T) {
			apiUser := &user.DBUser{
				Id:      "api_user",
				APIKey:  "api_key",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser.Insert())
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      "new_" + apiUser.APIKey,
				Roles:    []string{"role"},
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
			require.NoError(t, err)
			assert.NotNil(t, um)

			checkUser(t, u)
		},
		"UpdateExistingNotOnlyAPIUserFails": func(t *testing.T) {
			regUser := &user.DBUser{
				Id:     "reg_user",
				APIKey: "key",
			}
			require.NoError(t, regUser.Insert())
			u1 := evergreen.OnlyAPIUser{
				Username: "reg_user",
				Key:      "new_key",
				Roles:    []string{"role"},
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u1},
			}
			um, err := NewOnlyAPIUserManager(conf)
			assert.NoError(t, err)
			assert.NotNil(t, um)
		},
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
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
		"SynchronizesListOfChangedUsers": func(t *testing.T) {
			apiUser1 := &user.DBUser{
				Id:      "api_user1",
				APIKey:  "api_key1",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser1.Insert())
			apiUser2 := &user.DBUser{
				Id:      "api_user2",
				APIKey:  "api_key2",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser2.Insert())
			apiUser3 := &user.DBUser{
				Id:      "api_user3",
				APIKey:  "api_key3",
				OnlyAPI: true,
			}
			require.NoError(t, apiUser3.Insert())
			regUser1 := &user.DBUser{
				Id:     "reg_user1",
				APIKey: "reg_key1",
			}
			require.NoError(t, regUser1.Insert())
			u1 := evergreen.OnlyAPIUser{
				Username: apiUser1.Id,
				Key:      apiUser1.APIKey,
			}
			u2 := evergreen.OnlyAPIUser{
				Username: apiUser2.Id,
				Key:      "new_" + apiUser2.APIKey,
			}
			u4 := evergreen.OnlyAPIUser{
				Username: "api_user4",
				Key:      "api_key4",
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u1, u2, u4},
			}

			um, err := NewOnlyAPIUserManager(conf)
			require.NoError(t, err)
			assert.NotNil(t, um)

			checkAPIUser1, err := user.FindOneById(u1.Username)
			require.NoError(t, err)
			assert.Equal(t, apiUser1.APIKey, checkAPIUser1.APIKey, "API-only user should be unchanged")

			checkAPIUser2, err := user.FindOneById(u2.Username)
			require.NoError(t, err)
			assert.Equal(t, u2.Key, checkAPIUser2.APIKey, "API-only user should be updated")

			checkAPIUser3, err := user.FindOneById(apiUser3.Id)
			assert.NoError(t, err)
			assert.Nil(t, checkAPIUser3, "old API-only user should be deleted from the DB")

			checkRegUser1, err := user.FindOneById(regUser1.Id)
			require.NoError(t, err)
			assert.Equal(t, regUser1.APIKey, checkRegUser1.APIKey, "regular user should not be changed")

			checkAPIUser5, err := user.FindOneById(u4.Username)
			require.NoError(t, err)
			assert.Equal(t, u4.Key, checkAPIUser5.APIKey, "new API-only user should be inserted")
		},
		"GetUserByIDSucceeds": func(t *testing.T) {
			u1 := evergreen.OnlyAPIUser{
				Username: "user1",
				Key:      "key1",
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u1},
			}
			um, err := NewOnlyAPIUserManager(conf)
			require.NoError(t, err)
			require.NotNil(t, um)

			checkUser, err := um.GetUserByID(u1.Username)
			require.NoError(t, err)
			checkDBUser, ok := checkUser.(*user.DBUser)
			require.True(t, ok, "user manager in evergreen must return DBUser")
			assert.Equal(t, u1.Key, checkDBUser.APIKey)
			assert.True(t, checkDBUser.OnlyAPI)
		},
		"GetUserByIDFailsIfNonexistent": func(t *testing.T) {
			u := evergreen.OnlyAPIUser{
				Username: "user1",
				Key:      "key1",
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      apiUser.APIKey,
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      apiUser.APIKey,
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      apiUser.APIKey,
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      apiUser.APIKey,
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      apiUser.APIKey,
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
			u := evergreen.OnlyAPIUser{
				Username: apiUser.Id,
				Key:      apiUser.APIKey,
			}
			conf := &evergreen.OnlyAPIAuthConfig{
				Users: []evergreen.OnlyAPIUser{u},
			}
			um, err := NewOnlyAPIUserManager(conf)
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
