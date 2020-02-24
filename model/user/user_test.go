package user

import (
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

var userTestConfig = testutil.TestConfig()

type UserTestSuite struct {
	suite.Suite
	users []*DBUser
}

func TestDBUser(t *testing.T) {
	s := &UserTestSuite{}
	suite.Run(t, s)
}

func (s *UserTestSuite) SetupSuite() {
	rm := evergreen.GetEnvironment().RoleManager()
	s.NoError(rm.RegisterPermissions([]string{"permission"}))
}

func (s *UserTestSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
	_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": evergreen.ScopeCollection})
	s.users = []*DBUser{
		&DBUser{
			Id:     "Test1",
			APIKey: "12345",
			Settings: UserSettings{
				GithubUser: GithubUser{
					UID:         1234,
					LastKnownAs: "octocat",
				},
			},
			LoginCache: LoginCache{
				AccessToken:  "access_token",
				RefreshToken: "refresh_token",
				Token:        "1234",
				TTL:          time.Now(),
			},
		},
		&DBUser{
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
		&DBUser{
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
		&DBUser{
			Id: "Test4",
			LoginCache: LoginCache{
				Token: "5678",
				TTL:   time.Now(),
			},
		},
		&DBUser{
			Id:     "Test5",
			APIKey: "api",
			LoginCache: LoginCache{
				Token:        "token5",
				AccessToken:  "access5",
				RefreshToken: "refresh5",
				TTL:          time.Now(),
			},
		},
		&DBUser{
			Id:     "Test6",
			APIKey: "api",
			LoginCache: LoginCache{
				Token:          "token6",
				AccessToken:    "access6",
				RefreshToken:   "refresh6",
				TTL:            time.Now().Add(-time.Hour),
				ReauthAttempts: 5,
			},
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

func (s *UserTestSuite) TestClearLoginCacheSingleUser() {
	// Error on non-existent user
	s.Error(ClearLoginCache(&DBUser{Id: "asdf"}, false))

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
	s.NoError(ClearLoginCache(u1, false))
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

func (s *UserTestSuite) TestClearLoginCacheAllUsers() {
	// Clear all users
	s.NoError(ClearLoginCache(nil, true))
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
		left, right := util.StringSliceSymmetricDifference(foundNames, names)
		return len(left) == 0 && len(right) == 0
	}

	users, err := FindNeedsReauthorization(0, 100)
	s.NoError(err)
	s.Len(users, 5)
	s.True(containsUsers(users, "Test1", "Test2", "Test4", "Test5", "Test6"), "should find all logged in users")
	s.False(containsUsers(users, "Test3"), "should not find logged out users")

	users, err = FindNeedsReauthorization(0, 1)
	s.NoError(err)
	s.Len(users, 4)
	s.True(containsUsers(users, "Test1", "Test2", "Test4", "Test5"), "should find logged in users who have not exceeded max reauth attempts")
	s.False(containsUsers(users, "Test3", "Test6"), "should not find logged out users or users who have exceeded max reauth attempts")

	users, err = FindNeedsReauthorization(30*time.Minute, 100)
	s.NoError(err)
	s.Len(users, 2)
	s.True(containsUsers(users, "Test2", "Test6"), "should find logged in users who have exceeded the reauth limit")

	users, err = FindNeedsReauthorization(24*time.Hour, 1)
	s.NoError(err)
	s.Empty(users, "should not find users who have not exceeded the reauth limit")
}
