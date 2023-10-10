package user

import (
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	testutil.Setup()
}

type UserTestSuite struct {
	suite.Suite
	users []*DBUser
}

func TestUserTestSuite(t *testing.T) {
	s := &UserTestSuite{}
	suite.Run(t, s)
}

func (s *UserTestSuite) SetupSuite() {
	rm := evergreen.GetEnvironment().RoleManager()
	s.NoError(rm.RegisterPermissions([]string{"permission"}))
}

func (s *UserTestSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	s.Require().NoError(db.CreateCollections(evergreen.ScopeCollection))
	s.users = []*DBUser{
		{
			Id:     "Test1",
			APIKey: "12345",
			Settings: UserSettings{
				SlackUsername: "person",
				SlackMemberId: "12345member",
				GithubUser: GithubUser{
					UID:         1234,
					LastKnownAs: "octocat",
				},
				UseSpruceOptions: UseSpruceOptions{
					SpruceV1: true,
				},
			},
			LoginCache: LoginCache{
				AccessToken:  "access_token",
				RefreshToken: "refresh_token",
				Token:        "1234",
				TTL:          time.Now(),
			},
		},
		{
			Id: "Test2",
			PubKeys: []PubKey{
				{
					Name:      "key1",
					Key:       "ssh-mock 12345",
					CreatedAt: time.Now(),
				},
			},
			APIKey: "67890",
			LoginCache: LoginCache{
				Token: "4321",
				TTL:   time.Now().Add(-time.Hour),
			},
		},
		{
			Id: "Test3",
			PubKeys: []PubKey{
				{
					Name:      "key1",
					Key:       "ssh-mock 12345",
					CreatedAt: time.Now(),
				},
			},
			APIKey: "67890",
		},
		{
			Id: "Test4",
			LoginCache: LoginCache{
				Token: "5678",
				TTL:   time.Now(),
			},
		},
		{
			Id:     "Test5",
			APIKey: "api",
			LoginCache: LoginCache{
				Token:        "token5",
				AccessToken:  "access5",
				RefreshToken: "refresh5",
				TTL:          time.Now(),
			},
		},
		{
			Id: "Test6",
			PubKeys: []PubKey{
				{
					Name:      "key1",
					Key:       "ssh-mock 12345",
					CreatedAt: time.Now(),
				},
			},
			APIKey: "67833",
		},
	}

	for _, user := range s.users {
		s.NoError(user.Insert())
	}

	rm := evergreen.GetEnvironment().RoleManager()
	scope1 := gimlet.Scope{
		ID:          "1",
		Resources:   []string{"resource1", "resource2"},
		ParentScope: "3",
	}
	s.NoError(rm.AddScope(scope1))
	scope2 := gimlet.Scope{
		ID:          "2",
		Resources:   []string{"resource3"},
		ParentScope: "3",
	}
	s.NoError(rm.AddScope(scope2))
	scope3 := gimlet.Scope{
		ID:          "3",
		ParentScope: "root",
	}
	s.NoError(rm.AddScope(scope3))
	scope4 := gimlet.Scope{
		ID:          "4",
		Resources:   []string{"resource4"},
		ParentScope: "root",
	}
	s.NoError(rm.AddScope(scope4))
	root := gimlet.Scope{
		ID:        "root",
		Resources: []string{"resource1", "resource2", "resource3", "resource4"},
	}
	s.NoError(rm.AddScope(root))
	r1p1 := gimlet.Role{
		ID:    "r1p1",
		Scope: "1",
		Permissions: map[string]int{
			"permission": 1,
		},
	}
	s.NoError(rm.UpdateRole(r1p1))
	r2p2 := gimlet.Role{
		ID:    "r2p2",
		Scope: "2",
		Permissions: map[string]int{
			"permission": 2,
		},
	}
	s.NoError(rm.UpdateRole(r2p2))
	r12p1 := gimlet.Role{
		ID:    "r12p1",
		Scope: "3",
		Permissions: map[string]int{
			"permission": 1,
		},
	}
	s.NoError(rm.UpdateRole(r12p1))
	r4p1 := gimlet.Role{
		ID:    "r4p1",
		Scope: "4",
		Permissions: map[string]int{
			"permission": 1,
		},
	}
	s.NoError(rm.UpdateRole(r4p1))
	r1234p2 := gimlet.Role{
		ID:    "r1234p2",
		Scope: "root",
		Permissions: map[string]int{
			"permission": 2,
		},
	}
	s.NoError(rm.UpdateRole(r1234p2))
}

func (s *UserTestSuite) TearDownTest() {
	s.NoError(db.ClearCollections(Collection))
}

func (s *UserTestSuite) TestGetPublicKey() {
	key, err := s.users[1].GetPublicKey("key1")
	s.NoError(err)
	s.Equal(key, "ssh-mock 12345")
}

func (s *UserTestSuite) TestGetPublicKeyThatDoesntExist() {
	key, err := s.users[1].GetPublicKey("key2thatdoesntexist")
	s.Error(err)
	s.Empty(key)
}

func (s *UserTestSuite) TestAddKey() {
	s.Require().NoError(s.users[0].AddPublicKey("key1", "ssh-mock 67890"))
	key, err := s.users[0].GetPublicKey("key1")
	s.Require().NoError(err)
	s.Equal(key, "ssh-mock 67890")

	u, err := FindOne(ById(s.users[0].Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)

	s.checkUserNotDestroyed(u, s.users[0])

	s.Equal(u.PubKeys[0].Name, "key1")
	s.Equal(u.PubKeys[0].Key, "ssh-mock 67890")
}

func (s *UserTestSuite) TestAddDuplicateKeyFails() {
	err := s.users[1].AddPublicKey("key1", "ssh-mock 67890")
	s.Error(err)
	s.Contains(err.Error(), "not found")

	u, err := FindOne(ById(s.users[1].Id))
	s.NoError(err)
	s.NotNil(u)
	s.checkUserNotDestroyed(u, s.users[1])
}

func (s *UserTestSuite) checkUserNotDestroyed(fromDB *DBUser, expected *DBUser) {
	s.Equal(fromDB.Id, expected.Id)
	s.Equal(fromDB.APIKey, expected.APIKey)
}

func (s *UserTestSuite) TestUpdatePublicKey() {
	s.NoError(s.users[5].UpdatePublicKey("key1", "key1", "this is an amazing key"))
	s.Len(s.users[5].PubKeys, 1)
	s.Contains(s.users[5].PubKeys[0].Name, "key1")
	s.Contains(s.users[5].PubKeys[0].Key, "this is an amazing key")

	u, err := FindOne(ById(s.users[5].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[5])
}

func (s *UserTestSuite) TestUpdatePublicKeyWithSameKeyName() {
	s.NoError(s.users[5].UpdatePublicKey("key1", "keyAmazing", "this is an amazing key"))
	s.Len(s.users[5].PubKeys, 1)
	s.Contains(s.users[5].PubKeys[0].Name, "keyAmazing")
	s.Contains(s.users[5].PubKeys[0].Key, "this is an amazing key")

	u, err := FindOne(ById(s.users[5].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[5])
}

func (s *UserTestSuite) TestUpdatePublicKeyThatDoesntExist() {
	s.Error(s.users[5].UpdatePublicKey("non-existent-key", "keyAmazing", "this is an amazing key"))
	s.Len(s.users[5].PubKeys, 1)
	s.Contains(s.users[5].PubKeys[0].Name, "key1")
	s.Contains(s.users[5].PubKeys[0].Key, "ssh-mock 12345")

	u, err := FindOne(ById(s.users[5].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[5])
}

func (s *UserTestSuite) TestDeletePublicKey() {
	s.NoError(s.users[1].DeletePublicKey("key1"))
	s.Len(s.users[1].PubKeys, 0)
	s.Equal("67890", s.users[1].APIKey)

	u, err := FindOne(ById(s.users[1].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[1])
}

func (s *UserTestSuite) TestDeletePublicKeyThatDoesntExist() {
	s.Error(s.users[0].DeletePublicKey("key1"))
	s.Len(s.users[0].PubKeys, 0)
	s.Equal("12345", s.users[0].APIKey)

	u, err := FindOne(ById(s.users[0].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[0])
}

func (s *UserTestSuite) TestFindByGithubUID() {
	u, err := FindByGithubUID(1234)
	s.NoError(err)
	s.Equal("Test1", u.Id)

	u, err = FindByGithubUID(0)
	s.NoError(err)
	s.Nil(u)

	u, err = FindByGithubUID(-1)
	s.NoError(err)
	s.Nil(u)
}

func (s *UserTestSuite) TestFindOneByToken() {
	u, err := FindOneByToken("1234")
	s.NoError(err)
	s.NotNil(u)
	s.Equal("Test1", u.Id)

	u, err = FindOneByToken("4321")
	s.NoError(err)
	s.NotNil(u)
	s.Equal("Test2", u.Id)

	u, err = FindOneByToken("1111")
	s.NoError(err)
	s.Nil(u)
}

func (s *UserTestSuite) TestFindOneById() {
	u, err := FindOneById(s.users[0].Id)
	s.NoError(err)
	s.NotNil(u)
	s.Equal("Test1", u.Id)

	u, err = FindOneById(s.users[1].Id)
	s.NoError(err)
	s.NotNil(u)
	s.Equal("Test2", u.Id)

	u, err = FindOneByToken("1111")
	s.NoError(err)
	s.Nil(u)
}

func (s *UserTestSuite) TestPutLoginCache() {
	token1, err := PutLoginCache(s.users[0])
	s.NoError(err)
	s.NotEmpty(token1)

	token2, err := PutLoginCache(s.users[1])
	s.NoError(err)
	s.NotEmpty(token2)

	token3, err := PutLoginCache(&DBUser{Id: "asdf"})
	s.Error(err)
	s.Empty(token3)

	u1, err := FindOneById(s.users[0].Id)
	s.NoError(err)
	s.Equal(s.users[0].Id, u1.Id)
	s.Equal(s.users[0].LoginCache.AccessToken, u1.LoginCache.AccessToken)
	s.Equal(s.users[0].LoginCache.RefreshToken, u1.LoginCache.RefreshToken)

	u2, err := FindOneById(s.users[1].Id)
	s.NoError(err)
	s.Equal(s.users[1].Id, u2.Id)
	s.Equal(s.users[0].LoginCache.AccessToken, u1.LoginCache.AccessToken)
	s.Equal(s.users[0].LoginCache.RefreshToken, u1.LoginCache.RefreshToken)

	s.NotEqual(u1.LoginCache.Token, u2.LoginCache.Token)
	s.WithinDuration(time.Now(), u1.LoginCache.TTL, time.Second)
	s.WithinDuration(time.Now(), u2.LoginCache.TTL, time.Second)

	// Put to first user again, ensuring token stays the same but TTL changes
	time.Sleep(time.Millisecond) // sleep to check TTL changed
	token4, err := PutLoginCache(s.users[0])
	s.NoError(err)
	u4, err := FindOneById(s.users[0].Id)
	s.NoError(err)
	s.Equal(u1.LoginCache.Token, u4.LoginCache.Token)
	s.NotEqual(u1.LoginCache.TTL, u4.LoginCache.TTL)
	s.Equal(token1, token4)

	// Change access and refresh tokens, which should update
	s.users[0].LoginCache.AccessToken = "new_access_token"
	s.users[0].LoginCache.RefreshToken = "new_refresh_token"
	token5, err := PutLoginCache(s.users[0])
	s.NoError(err)
	u5, err := FindOneById(s.users[0].Id)
	s.NoError(err)
	s.Equal(u1.LoginCache.Token, u5.LoginCache.Token)
	s.Equal(token1, token5)
	s.Equal(s.users[0].LoginCache.AccessToken, u5.LoginCache.AccessToken)
	s.Equal(s.users[0].LoginCache.RefreshToken, u5.LoginCache.RefreshToken)

	// Fresh user with no token should generate new token
	token6, err := PutLoginCache(s.users[2])
	s.NoError(err)
	u6, err := FindOneById(s.users[2].Id)
	s.Equal(token6, u6.LoginCache.Token)
	s.NoError(err)
	s.NotEmpty(token6)
	s.NotEqual(token1, token6)
	s.NotEqual(token2, token6)
	s.NotEqual(token3, token6)
	s.NotEqual(token4, token6)
	s.NotEqual(token5, token6)
}

func (s *UserTestSuite) TestGetLoginCache() {
	u, valid, err := GetLoginCache("1234", time.Minute)
	s.NoError(err)
	s.True(valid)
	s.Equal("Test1", u.Username())

	u, valid, err = GetLoginCache("4321", time.Minute)
	s.NoError(err)
	s.False(valid)
	s.Equal("Test2", u.Username())

	u, valid, err = GetLoginCache("asdf", time.Minute)
	s.NoError(err)
	s.False(valid)
	s.Nil(u)
}

func (s *UserTestSuite) TestClearLoginCache() {
	// Error on non-existent user
	s.Error(ClearLoginCache(&DBUser{Id: "asdf"}))

	// Two valid users...
	u1, valid, err := GetLoginCache("1234", time.Minute)
	s.Require().NoError(err)
	s.Require().True(valid)
	s.Require().Equal("Test1", u1.Username())
	u2, valid, err := GetLoginCache("5678", time.Minute)
	s.Require().NoError(err)
	s.Require().True(valid)
	s.Require().Equal("Test4", u2.Username())

	// One is cleared...
	s.NoError(ClearLoginCache(u1))
	// and is no longer found
	u1, valid, err = GetLoginCache("1234", time.Minute)
	s.NoError(err)
	s.False(valid)
	s.Nil(u1)

	// The other user remains
	u2, valid, err = GetLoginCache("5678", time.Minute)
	s.NoError(err)
	s.True(valid)
	s.Equal("Test4", u2.Username())
}

func (s *UserTestSuite) TestClearAllLoginCaches() {
	// Clear all users
	s.NoError(ClearAllLoginCaches())
	// Sample user is no longer in cache
	u, valid, err := GetLoginCache("1234", time.Minute)
	s.NoError(err)
	s.False(valid)
	s.Nil(u)
}

func (s *UserTestSuite) TestGetPatchUser() {
	uid := 1234
	u, err := GetPatchUser(uid)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal("Test1", u.Id)

	uid = 9876
	u, err = GetPatchUser(uid)
	s.NoError(err)
	s.NotNil(u)
	s.Equal(evergreen.GithubPatchUser, u.Id)
}

func (s *UserTestSuite) TestGetSlackMemberId() {
	username := "person"
	memberId, err := GetSlackMemberId(username)
	s.NoError(err)
	s.Equal("12345member", memberId)
}

func (s *UserTestSuite) TestRoles() {
	u := s.users[0]
	for i := 1; i <= 3; i++ {
		s.NoError(u.AddRole(strconv.Itoa(i)))
	}
	dbUser, err := FindOneById(u.Id)
	s.NoError(err)
	s.EqualValues(dbUser.SystemRoles, u.SystemRoles)

	s.NoError(u.RemoveRole("2"))
	dbUser, err = FindOneById(u.Id)
	s.NoError(err)
	s.EqualValues(dbUser.SystemRoles, u.SystemRoles)
	s.NoError(u.RemoveRole("definitely non-existent role"))
}

func (s *UserTestSuite) TestFavoriteProjects() {
	u := s.users[0]
	projID := "annie-copy5"
	expected := []string{projID}

	// add a project
	err := u.AddFavoritedProject(projID)
	s.NoError(err)
	s.EqualValues(u.FavoriteProjects, expected)

	// try to add the same project again
	err = u.AddFavoritedProject(projID)
	s.Require().Error(err)
	s.EqualValues(u.FavoriteProjects, expected)

	// remove a project
	expected = []string{}

	err = u.RemoveFavoriteProject(projID)
	s.NoError(err)
	s.EqualValues(u.FavoriteProjects, expected)

	// try to remove a project that does not exist
	err = u.RemoveFavoriteProject(projID)
	s.Require().Error(err)
	s.EqualValues(u.FavoriteProjects, expected)
}

func (s *UserTestSuite) TestHasPermission() {
	u := s.users[0]

	// no roles - no permission
	hasPermission := u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 1})
	s.False(hasPermission)

	// has a role with explicit permission to the resource
	s.NoError(u.AddRole("r1p1"))
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 1})
	s.True(hasPermission)
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 0})
	s.True(hasPermission)

	// role with insufficient permission but the right resource
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 2})
	s.False(hasPermission)

	// role with a parent scope
	s.NoError(u.AddRole("r12p1"))
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource2", Permission: "permission", RequiredLevel: 1})
	s.True(hasPermission)

	// role with no permission to the specified resource
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource4", Permission: "permission", RequiredLevel: 1})
	s.False(hasPermission)

	// permission to everything
	s.NoError(u.RemoveRole("r1p1"))
	s.NoError(u.RemoveRole("r12p1"))
	s.NoError(u.AddRole("r1234p2"))
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource4", Permission: "permission", RequiredLevel: 2})
	s.True(hasPermission)
}

func (s *UserTestSuite) TestFindNeedsReauthorization() {
	containsUsers := func(users []DBUser, names ...string) bool {
		foundNames := make([]string, 0, len(users))
		for _, u := range users {
			foundNames = append(foundNames, u.Username())
		}
		left, right := utility.StringSliceSymmetricDifference(foundNames, names)
		return len(left) == 0 && len(right) == 0
	}

	users, err := FindNeedsReauthorization(0)
	s.NoError(err)
	s.Require().Len(users, 4)
	s.True(containsUsers(users, "Test1", "Test2", "Test4", "Test5"), "should find all logged in users")
	s.False(containsUsers(users, "Test3"), "should not find logged out users")

	users, err = FindNeedsReauthorization(0)
	s.NoError(err)
	s.Require().Len(users, 4)
	s.True(containsUsers(users, "Test1", "Test2", "Test4", "Test5"), "should find logged in users who have not exceeded max reauth attempts")
	s.False(containsUsers(users, "Test3", "Test6"), "should not find logged out users or users who have exceeded max reauth attempts")

	users, err = FindNeedsReauthorization(30 * time.Minute)
	s.NoError(err)
	s.Require().Len(users, 1)
	s.True(containsUsers(users, "Test2"), "should find logged in users who have exceeded the reauth limit")

	users, err = FindNeedsReauthorization(24 * time.Hour)
	s.NoError(err)
	s.Empty(users, "should not find users who have not exceeded the reauth limit")
}

func TestServiceUserOperations(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	u := DBUser{
		Id:          "u",
		DispName:    "service_user",
		SystemRoles: []string{"one"},
	}
	assert.EqualError(t, AddOrUpdateServiceUser(u), "cannot update a non-service user")
	u.OnlyAPI = true
	assert.NoError(t, AddOrUpdateServiceUser(u))
	dbUser, err := FindOneById(u.Id)
	assert.NoError(t, err)
	assert.True(t, dbUser.OnlyAPI)
	assert.Equal(t, u.DispName, dbUser.DispName)
	assert.Equal(t, u.SystemRoles, dbUser.SystemRoles)
	u.APIKey = dbUser.APIKey
	assert.NotEmpty(t, u.APIKey)

	u.DispName = "another"
	u.SystemRoles = []string{"one", "two"}
	assert.NoError(t, AddOrUpdateServiceUser(u))
	dbUser, err = FindOneById(u.Id)
	assert.NoError(t, err)
	assert.True(t, dbUser.OnlyAPI)
	assert.Equal(t, u.DispName, dbUser.DispName)
	assert.Equal(t, u.SystemRoles, dbUser.SystemRoles)
	assert.Equal(t, u.APIKey, dbUser.APIKey)

	users, err := FindServiceUsers()
	assert.NoError(t, err)
	assert.Len(t, users, 1)

	err = DeleteServiceUser("doesntexist")
	assert.EqualError(t, err, "service user 'doesntexist' not found")
	err = DeleteServiceUser(u.Id)
	assert.NoError(t, err)
	dbUser, err = FindOneById(u.Id)
	assert.NoError(t, err)
	assert.Nil(t, dbUser)
}
func TestGetOrCreateUser(t *testing.T) {
	id := "id"
	name := "name"
	email := ""
	accessToken := "access_token"
	refreshToken := "refresh_token"

	checkUser := func(t *testing.T, user *DBUser, id, name, email, accessToken, refreshToken string) {
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

			dbUser, err := FindOneById(id)
			require.NoError(t, err)
			require.NotZero(t, dbUser)
			checkUser(t, dbUser, id, name, email, accessToken, refreshToken)
			assert.Equal(t, apiKey, dbUser.GetAPIKey())
		},
		"UpdateAlwaysSetsName": func(t *testing.T) {
			user, err := GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			apiKey := user.GetAPIKey()
			assert.NotEmpty(t, apiKey)

			newName := "new_name"
			user, err = GetOrCreateUser(id, newName, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, newName, email, accessToken, refreshToken)
			assert.Equal(t, apiKey, user.GetAPIKey())
		},
		"UpdateSetsRefreshTokenIfNonempty": func(t *testing.T) {
			user, err := GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			apiKey := user.GetAPIKey()
			assert.NotEmpty(t, apiKey)

			user, err = GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			require.NoError(t, err)

			checkUser(t, user, id, name, email, accessToken, refreshToken)
			assert.Equal(t, apiKey, user.GetAPIKey())

			user, err = GetOrCreateUser(id, name, email, accessToken, "", nil)
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
			user, err := GetOrCreateUser(id, "", "", "token", "", roles)
			assert.NoError(t, err)
			checkUser(t, user, id, "id", "", "token", "")
			assert.Equal(t, roles, user.Roles())

			user, err = GetOrCreateUser(id, name, email, accessToken, refreshToken, nil)
			assert.NoError(t, err)
			checkUser(t, user, id, name, email, accessToken, refreshToken)
			assert.Equal(t, roles, user.Roles())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			testCase(t)
		})
	}
}

func TestGeneralSubscriptionIDs(t *testing.T) {
	u := DBUser{}
	u.Settings.Notifications = NotificationPreferences{
		BuildBreakID:          "BuildBreakID",
		PatchFinishID:         "PatchFinishID",
		PatchFirstFailureID:   "PatchFirstFailureID",
		SpawnHostExpirationID: "SpawnHostExpirationID",
		SpawnHostOutcomeID:    "SpawnHostOutcomeID",
		CommitQueueID:         "CommitQueueID",
	}

	assert.ElementsMatch(t, []string{
		"BuildBreakID",
		"PatchFinishID",
		"PatchFirstFailureID",
		"SpawnHostExpirationID",
		"SpawnHostOutcomeID",
		"CommitQueueID",
	}, u.GeneralSubscriptionIDs())
}

func TestViewableProjectSettings(t *testing.T) {
	rm := evergreen.GetEnvironment().RoleManager()
	assert.NoError(t, db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, Collection))
	editScope := gimlet.Scope{
		ID:        "edit_scope",
		Resources: []string{"edit1", "edit2"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(t, rm.AddScope(editScope))
	editPermissions := gimlet.Permissions{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
	}
	editRole := gimlet.Role{
		ID:          "edit_role",
		Scope:       editScope.ID,
		Permissions: editPermissions,
	}
	assert.NoError(t, rm.UpdateRole(editRole))
	viewScope := gimlet.Scope{
		ID:        "view_scope",
		Resources: []string{"view1"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(t, rm.AddScope(viewScope))
	viewPermissions := gimlet.Permissions{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
	}
	viewRole := gimlet.Role{
		ID:          "view_role",
		Scope:       viewScope.ID,
		Permissions: viewPermissions,
	}
	assert.NoError(t, rm.UpdateRole(viewRole))
	otherScope := gimlet.Scope{
		ID:        "others_scope",
		Resources: []string{"other"},
		Type:      evergreen.ProjectResourceType,
	}
	assert.NoError(t, rm.AddScope(otherScope))

	otherRole := gimlet.Role{
		ID:          "other_role",
		Scope:       otherScope.ID,
		Permissions: gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsNone.Value},
	}
	assert.NoError(t, rm.UpdateRole(otherRole))
	myUser := DBUser{
		Id: "me",
	}
	assert.NoError(t, myUser.Insert())
	assert.NoError(t, myUser.AddRole(viewRole.ID))
	assert.NoError(t, myUser.AddRole(editRole.ID))
	assert.NoError(t, myUser.AddRole(otherRole.ID))

	// assert that viewable projects contains the edit projects and the view projects
	projects, err := myUser.GetViewableProjectSettings()
	assert.NoError(t, err)
	assert.Len(t, projects, 3)
	assert.Contains(t, projects, "edit1")
	assert.Contains(t, projects, "edit2")
	assert.Contains(t, projects, "view1")
	assert.NotContains(t, projects, "other")
}

func (s *UserTestSuite) TestClearUser() {
	// Error on non-existent user.
	s.Error(ClearUser("asdf"))

	u, err := FindOneById(s.users[0].Id)
	s.NoError(err)
	s.NotNil(u)
	s.NotEmpty(u.Settings)
	s.Equal("Test1", u.Id)
	s.Equal(true, u.Settings.UseSpruceOptions.SpruceV1)
	s.NoError(u.AddRole("r1p1"))
	s.NotEmpty(u.SystemRoles)

	s.NoError(ClearUser(u.Id))

	// Sensitive settings and roles should now be empty.
	u, err = FindOneById(s.users[0].Id)
	s.NoError(err)
	s.NotNil(u)

	s.Empty(u.Settings.GithubUser)
	s.Empty(u.Settings.SlackUsername)
	s.Empty(u.Settings.SlackMemberId)
	s.Empty(u.Roles())
	s.Empty(u.LoginCache)

	// User should have spruce UI enabled.
	s.True(u.Settings.UseSpruceOptions.SpruceV1)

	// Should enable for user that previously had it false
	u, err = FindOneById(s.users[1].Id)
	s.NoError(err)
	s.NotNil(u)
	s.False(u.Settings.UseSpruceOptions.SpruceV1)
	s.NoError(ClearUser(u.Id))
	u, err = FindOneById(s.users[1].Id)
	s.NoError(err)
	s.True(u.Settings.UseSpruceOptions.SpruceV1)
}
