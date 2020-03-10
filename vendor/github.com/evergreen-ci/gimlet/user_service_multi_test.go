package gimlet

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiUserManager(t *testing.T) {
	makeUser := func(n int) *MockUser {
		ns := fmt.Sprint(n)
		return &MockUser{
			ID:       "user" + ns,
			Password: "password" + ns,
			Token:    "token" + ns,
			Groups:   []string{"group" + ns},
		}
	}
	readWrite := func() *MockUserManager {
		return &MockUserManager{
			Users:                []*MockUser{makeUser(1), makeUser(2)},
			Redirect:             true,
			LoginHandler:         func(w http.ResponseWriter, r *http.Request) {},
			LoginCallbackHandler: func(w http.ResponseWriter, r *http.Request) {},
		}
	}
	readOnly := func() *MockUserManager {
		return &MockUserManager{
			Users:                []*MockUser{makeUser(1), makeUser(3)},
			Redirect:             true,
			LoginHandler:         func(http.ResponseWriter, *http.Request) {},
			LoginCallbackHandler: func(http.ResponseWriter, *http.Request) {},
		}
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager){
		"GetUserByTokenSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			u, err := um.GetUserByToken(ctx, makeUser(2).Token)
			require.NoError(t, err)
			assert.Equal(t, makeUser(2).Username(), u.Username())
		},
		"GetUserByTokenFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByToken = true
			u, err := um.GetUserByToken(ctx, makeUser(2).Token)
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"GetUserByTokenNonexistentFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			u, err := um.GetUserByToken(ctx, makeUser(4).Token)
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"GetUserByTokenTriesAllManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByToken = true
			u, err := um.GetUserByToken(ctx, makeUser(1).Token)
			require.NoError(t, err)
			assert.Equal(t, makeUser(1).Username(), u.Username())
		},
		"GetUserByTokenTriesReadManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			u, err := um.GetUserByToken(ctx, makeUser(3).Token)
			require.NoError(t, err)
			assert.Equal(t, makeUser(3).Username(), u.Username())
		},
		"GetUserByTokenPrioritizesReadWriteManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readOnly.FailGetUserByToken = true
			u, err := um.GetUserByToken(ctx, makeUser(1).Token)
			require.NoError(t, err)
			assert.Equal(t, makeUser(1).Username(), u.Username())
		},
		"GetUserByTokenFailsIfAllManagersFail": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByToken = true
			readOnly.FailGetUserByToken = true
			u, err := um.GetUserByToken(ctx, makeUser(1).Token)
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"GetUserByIDSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			u, err := um.GetUserByID(makeUser(2).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(2).Username(), u.Username())
		},
		"GetUserByIDFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByID = true
			u, err := um.GetUserByID(makeUser(2).Username())
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"GetUserByIDNonexistentFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			u, err := um.GetUserByID(makeUser(4).Username())
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"GetUserByIDTriesAllManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByID = true
			u, err := um.GetUserByID(makeUser(1).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(1).Username(), u.Username())
		},
		"GetUserByIDTriesReadManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByID = true
			u, err := um.GetUserByID(makeUser(3).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(3).Username(), u.Username())
		},
		"GetUserByIDPrioritizesReadWriteManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readOnly.FailGetUserByID = true
			u, err := um.GetUserByID(makeUser(1).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(1).Username(), u.Username())
		},
		"GetUserByIDFailsIfAllManagersFail": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetUserByID = true
			readOnly.FailGetUserByID = true
			u, err := um.GetUserByID(makeUser(1).Username())
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"GetGroupsForUserSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			groups, err := um.GetGroupsForUser(makeUser(2).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(2).Groups, groups)
		},
		"GetGroupsForUserFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetGroupsForUser = true
			groups, err := um.GetGroupsForUser(makeUser(2).Username())
			assert.Error(t, err)
			assert.Empty(t, groups)
		},
		"GetGroupsForUserNonexistentFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			groups, err := um.GetGroupsForUser(makeUser(4).Username())
			assert.Error(t, err)
			assert.Empty(t, groups)
		},
		"GetGroupsForUserTriesAllManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetGroupsForUser = true
			groups, err := um.GetGroupsForUser(makeUser(1).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(1).Groups, groups)
		},
		"GetGroupsForUserTriesReadManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			groups, err := um.GetGroupsForUser(makeUser(3).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(3).Groups, groups)
		},
		"GetGroupsForUserPrioritizesReadWriteManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readOnly.FailGetGroupsForUser = true
			groups, err := um.GetGroupsForUser(makeUser(1).Username())
			require.NoError(t, err)
			assert.Equal(t, makeUser(1).Groups, groups)
		},
		"GetGroupsForUserFailsIfAllManagersFail": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailGetGroupsForUser = true
			readOnly.FailGetGroupsForUser = true
			u, err := um.GetGroupsForUser(makeUser(1).Username())
			assert.Error(t, err)
			assert.Empty(t, u)
		},
		"ReauthorizeUserSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			assert.NoError(t, um.ReauthorizeUser(makeUser(1)))
		},
		"ReauthorizeUserIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			assert.Error(t, um.ReauthorizeUser(makeUser(3)))
			readOnly.FailReauthorizeUser = true
			assert.NoError(t, um.ReauthorizeUser(makeUser(1)))
		},
		"ReauthorizeUserNonexistentFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			assert.Error(t, um.ReauthorizeUser(makeUser(4)))
		},
		"ReauthorizeUserFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.FailReauthorizeUser = true
			assert.Error(t, um.ReauthorizeUser(makeUser(1)))
		},
		"CreateUserTokenSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			token, err := um.CreateUserToken(user.Username(), user.Password)
			require.NoError(t, err)
			assert.Equal(t, mockUserToken(user.Username(), user.Password), token)
		},
		"CreateUserTokenIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			readOnly.FailCreateUserToken = true
			token, err := um.CreateUserToken(user.Username(), user.Password)
			require.NoError(t, err)
			assert.Equal(t, mockUserToken(user.Username(), user.Password), token)
		},
		"CreateUserTokenFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			readWrite.FailCreateUserToken = true
			token, err := um.CreateUserToken(user.Username(), user.Password)
			assert.Error(t, err)
			assert.Empty(t, token)
		},
		"GetOrCreateUserSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			u, err := um.GetOrCreateUser(user)
			require.NoError(t, err)
			assert.Equal(t, user.Username(), u.Username())
		},
		"GetOrCreateUserIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			readOnly.FailGetOrCreateUser = true
			u, err := um.GetOrCreateUser(user)
			require.NoError(t, err)
			assert.Equal(t, user.Username(), u.Username())
		},
		"GetOrCreateUserFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			readWrite.FailGetOrCreateUser = true
			u, err := um.GetOrCreateUser(user)
			assert.Error(t, err)
			assert.Nil(t, u)
		},
		"ClearUserSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			assert.NoError(t, um.ClearUser(user, false))
		},
		"ClearUserIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			readOnly.FailClearUser = true
			assert.NoError(t, um.ClearUser(user, false))
		},
		"ClearUserFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			user := makeUser(4)
			readWrite.FailClearUser = true
			assert.Error(t, um.ClearUser(user, false))
		},
		"GetLoginHandlerSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			h := um.GetLoginHandler("")
			assert.NotNil(t, h)
		},
		"GetLoginHandlerIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readOnly.LoginHandler = nil
			h := um.GetLoginHandler("")
			assert.NotNil(t, h)
		},
		"GetLoginHandlerNil": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.LoginHandler = nil
			h := um.GetLoginHandler("")
			assert.Nil(t, h)
		},
		"GetLoginCallbackHandlerSucceeds": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			h := um.GetLoginCallbackHandler()
			assert.NotNil(t, h)
		},
		"GetLoginCallbackHandlerIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readOnly.LoginCallbackHandler = nil
			h := um.GetLoginCallbackHandler()
			assert.NotNil(t, h)
		},
		"GetLoginCallbackHandlerFails": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.LoginCallbackHandler = nil
			h := um.GetLoginCallbackHandler()
			assert.Nil(t, h)
		},
		"IsRedirectTrue": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			assert.True(t, um.IsRedirect())
		},
		"IsRedirectIgnoresReadOnlyUserManagers": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readOnly.Redirect = false
			assert.True(t, um.IsRedirect())
		},
		"IsRedirectFalse": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {
			readWrite.Redirect = false
			assert.False(t, um.IsRedirect())
		},
		// "": func(ctx context.Context, t *testing.T, um UserManager, readWrite *MockUserManager, readOnly *MockUserManager) {},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rw := readWrite()
			ro := readOnly()
			um := NewMultiUserManager([]UserManager{rw}, []UserManager{ro})
			testCase(ctx, t, um, rw, ro)
		})
	}
}
