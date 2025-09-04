package user

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	testutil.Setup()
}

type UserTestSuite struct {
	suite.Suite
	env   evergreen.Environment
	users []*DBUser
}

func TestUserTestSuite(t *testing.T) {
	s := &UserTestSuite{}
	suite.Run(t, s)
}

func (s *UserTestSuite) SetupSuite() {
	env := testutil.NewEnvironment(s.T().Context(), s.T())
	s.env = env
	s.NoError(env.RoleManager().RegisterPermissions([]string{"permission"}))
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
			LastScheduledTasksAt: time.Now().Add(-30 * time.Minute),
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
		s.NoError(user.Insert(s.T().Context()))
	}

	rm := s.env.RoleManager()
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
	s.NoError(db.ClearCollections(Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
}

func (s *UserTestSuite) TestGetPublicKey() {
	key, err := s.users[1].GetPublicKey("key1")
	s.NoError(err)
	s.Equal("ssh-mock 12345", key)
}

func (s *UserTestSuite) TestGetPublicKeyThatDoesntExist() {
	key, err := s.users[1].GetPublicKey("key2thatdoesntexist")
	s.Error(err)
	s.Empty(key)
}

func (s *UserTestSuite) TestAddKey() {
	// Add public key when there are no existing keys.
	s.Require().NoError(s.users[0].AddPublicKey(s.T().Context(), "key1", "ssh-mock 67890"))
	key, err := s.users[0].GetPublicKey("key1")
	s.Require().NoError(err)
	s.Equal("ssh-mock 67890", key)

	// Add public key when public keys is nil.
	s.users[0].PubKeys = nil
	s.Require().NoError(s.users[0].AddPublicKey(s.T().Context(), "key1", "ssh-mock 67890"))
	key, err = s.users[0].GetPublicKey("key1")
	s.Require().NoError(err)
	s.Equal("ssh-mock 67890", key)

	u, err := FindOneContext(s.T().Context(), ById(s.users[0].Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)

	s.checkUserNotDestroyed(u, s.users[0])

	s.Equal("key1", u.PubKeys[0].Name)
	s.Equal("ssh-mock 67890", u.PubKeys[0].Key)
}

func (s *UserTestSuite) TestCheckAndUpdateSchedulingLimit() {
	u := s.users[0]
	maxScheduledTasks := 100

	// Should not be able to go to a negative counter
	s.Require().NoError(u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 100, false))
	u, err := FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(0, u.NumScheduledPatchTasks)

	// Confirm scheduling tasks less than the limit is allowed
	s.Require().NoError(u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 99, true))
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(99, u.NumScheduledPatchTasks)

	// Confirm NumScheduledPatchTasks is unchanged and we receive an error after breaching the limit
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 1, true)
	s.Require().Error(err)
	s.Contains(err.Error(), fmt.Sprintf("user '%s' has scheduled %d out of %d allowed tasks in the past hour", u.Id, u.NumScheduledPatchTasks, maxScheduledTasks))
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(99, u.NumScheduledPatchTasks)

	// Confirm unscheduling one task brings the count-down to 98
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 1, false)
	s.Require().NoError(err)
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(98, u.NumScheduledPatchTasks)

	// Confirm that scheduling one more task is now possible
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 1, true)
	s.Require().NoError(err)
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(99, u.NumScheduledPatchTasks)

	// When the last time the user has scheduled tasks falls out of the hour, we should reset the
	// counter, and we should not be able to go negative
	u.LastScheduledTasksAt = time.Now().Add(-1 * time.Hour)
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 5, true)
	s.Require().NoError(err)
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(5, u.NumScheduledPatchTasks)

	// When the last time the user has scheduled tasks falls out of the hour, we should reset the
	// counter, and we should not be able to go negative
	u.LastScheduledTasksAt = time.Now().Add(-1 * time.Hour)
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 5, false)
	s.Require().NoError(err)
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(0, u.NumScheduledPatchTasks)

	// Confirm you cannot schedule more tasks than the limit, even if your counter is zero
	u.LastScheduledTasksAt = time.Now().Add(-1 * time.Hour)
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 101, true)
	s.Require().Error(err)
	s.Contains(err.Error(), fmt.Sprintf("cannot schedule %d tasks, maximum hourly per-user limit is %d", 101, 100))
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(0, u.NumScheduledPatchTasks)

	// Confirm you can deactivate tasks even if you're already past the limit
	u.LastScheduledTasksAt = time.Now().Add(-1 * time.Minute)
	u.NumScheduledPatchTasks = 120
	update := bson.M{"$set": bson.M{NumScheduledPatchTasksKey: 120}}
	s.Require().NoError(UpdateOneContext(s.T().Context(), bson.M{IdKey: u.Id}, update))
	err = u.CheckAndUpdateSchedulingLimit(s.T().Context(), maxScheduledTasks, 10, false)
	s.Require().NoError(err)
	u, err = FindOneContext(s.T().Context(), ById(u.Id))
	s.Require().NoError(err)
	s.Require().NotNil(u)
	s.Equal(110, u.NumScheduledPatchTasks)
}

func (s *UserTestSuite) TestAddDuplicateKeyFails() {
	err := s.users[1].AddPublicKey(s.T().Context(), "key1", "ssh-mock 67890")
	s.Error(err)
	s.Contains(err.Error(), "not found")

	u, err := FindOneContext(s.T().Context(), ById(s.users[1].Id))
	s.NoError(err)
	s.NotNil(u)
	s.checkUserNotDestroyed(u, s.users[1])
}

func (s *UserTestSuite) checkUserNotDestroyed(fromDB *DBUser, expected *DBUser) {
	s.Equal(expected.Id, fromDB.Id)
	s.Equal(expected.APIKey, fromDB.APIKey)
}

func (s *UserTestSuite) TestUpdatePublicKey() {
	s.NoError(s.users[5].UpdatePublicKey(s.T().Context(), "key1", "key1", "this is an amazing key"))
	s.Len(s.users[5].PubKeys, 1)
	s.Contains(s.users[5].PubKeys[0].Name, "key1")
	s.Contains(s.users[5].PubKeys[0].Key, "this is an amazing key")

	u, err := FindOneContext(s.T().Context(), ById(s.users[5].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[5])
}

func (s *UserTestSuite) TestUpdatePublicKeyWithSameKeyName() {
	s.NoError(s.users[5].UpdatePublicKey(s.T().Context(), "key1", "keyAmazing", "this is an amazing key"))
	s.Len(s.users[5].PubKeys, 1)
	s.Contains(s.users[5].PubKeys[0].Name, "keyAmazing")
	s.Contains(s.users[5].PubKeys[0].Key, "this is an amazing key")

	u, err := FindOneContext(s.T().Context(), ById(s.users[5].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[5])
}

func (s *UserTestSuite) TestUpdatePublicKeyThatDoesntExist() {
	s.Error(s.users[5].UpdatePublicKey(s.T().Context(), "non-existent-key", "keyAmazing", "this is an amazing key"))
	s.Len(s.users[5].PubKeys, 1)
	s.Contains(s.users[5].PubKeys[0].Name, "key1")
	s.Contains(s.users[5].PubKeys[0].Key, "ssh-mock 12345")

	u, err := FindOneContext(s.T().Context(), ById(s.users[5].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[5])
}

func (s *UserTestSuite) TestDeletePublicKey() {
	s.NoError(s.users[1].DeletePublicKey(s.T().Context(), "key1"))
	s.Empty(s.users[1].PubKeys)
	s.Equal("67890", s.users[1].APIKey)

	u, err := FindOneContext(s.T().Context(), ById(s.users[1].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[1])
}

func (s *UserTestSuite) TestDeletePublicKeyThatDoesntExist() {
	s.Error(s.users[0].DeletePublicKey(s.T().Context(), "key1"))
	s.Empty(s.users[0].PubKeys)
	s.Equal("12345", s.users[0].APIKey)

	u, err := FindOneContext(s.T().Context(), ById(s.users[0].Id))
	s.NoError(err)
	s.checkUserNotDestroyed(u, s.users[0])
}

func (s *UserTestSuite) TestFindByGithubUID() {
	u, err := FindByGithubUID(s.T().Context(), 1234)
	s.NoError(err)
	s.Equal("Test1", u.Id)

	u, err = FindByGithubUID(s.T().Context(), 0)
	s.NoError(err)
	s.Nil(u)

	u, err = FindByGithubUID(s.T().Context(), -1)
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
	u, err := FindOneByIdContext(s.T().Context(), s.users[0].Id)
	s.NoError(err)
	s.NotNil(u)
	s.Equal("Test1", u.Id)

	u, err = FindOneByIdContext(s.T().Context(), s.users[1].Id)
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

	u1, err := FindOneByIdContext(s.T().Context(), s.users[0].Id)
	s.NoError(err)
	s.Equal(s.users[0].Id, u1.Id)
	s.Equal(s.users[0].LoginCache.AccessToken, u1.LoginCache.AccessToken)
	s.Equal(s.users[0].LoginCache.RefreshToken, u1.LoginCache.RefreshToken)

	u2, err := FindOneByIdContext(s.T().Context(), s.users[1].Id)
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
	u4, err := FindOneByIdContext(s.T().Context(), s.users[0].Id)
	s.NoError(err)
	s.Equal(u1.LoginCache.Token, u4.LoginCache.Token)
	s.NotEqual(u1.LoginCache.TTL, u4.LoginCache.TTL)
	s.Equal(token1, token4)

	// Change access and refresh tokens, which should update
	s.users[0].LoginCache.AccessToken = "new_access_token"
	s.users[0].LoginCache.RefreshToken = "new_refresh_token"
	token5, err := PutLoginCache(s.users[0])
	s.NoError(err)
	u5, err := FindOneByIdContext(s.T().Context(), s.users[0].Id)
	s.NoError(err)
	s.Equal(u1.LoginCache.Token, u5.LoginCache.Token)
	s.Equal(token1, token5)
	s.Equal(s.users[0].LoginCache.AccessToken, u5.LoginCache.AccessToken)
	s.Equal(s.users[0].LoginCache.RefreshToken, u5.LoginCache.RefreshToken)

	// Fresh user with no token should generate new token
	token6, err := PutLoginCache(s.users[2])
	s.NoError(err)
	u6, err := FindOneByIdContext(s.T().Context(), s.users[2].Id)
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

func (s *UserTestSuite) TestRoles() {
	u := s.users[0]
	for i := 1; i <= 3; i++ {
		s.NoError(u.AddRole(s.T().Context(), strconv.Itoa(i)))
	}
	dbUser, err := FindOneByIdContext(s.T().Context(), u.Id)
	s.NoError(err)
	s.EqualValues(dbUser.SystemRoles, u.SystemRoles)

	s.NoError(u.RemoveRole(s.T().Context(), "2"))
	dbUser, err = FindOneByIdContext(s.T().Context(), u.Id)
	s.NoError(err)
	s.EqualValues(dbUser.SystemRoles, u.SystemRoles)
	s.NoError(u.RemoveRole(s.T().Context(), "definitely non-existent role"))
}

func (s *UserTestSuite) TestFavoriteProjects() {
	u := s.users[0]
	projID := "annie-copy5"
	expected := []string{projID}

	// add a project
	err := u.AddFavoritedProject(s.T().Context(), projID)
	s.NoError(err)
	s.EqualValues(expected, u.FavoriteProjects)

	// try to add the same project again
	err = u.AddFavoritedProject(s.T().Context(), projID)
	s.Require().Error(err)
	s.EqualValues(expected, u.FavoriteProjects)

	// remove a project
	expected = []string{}

	err = u.RemoveFavoriteProject(s.T().Context(), projID)
	s.NoError(err)
	s.EqualValues(expected, u.FavoriteProjects)

	// try to remove a project that does not exist
	err = u.RemoveFavoriteProject(s.T().Context(), projID)
	s.Require().Error(err)
	s.EqualValues(expected, u.FavoriteProjects)
}

func (s *UserTestSuite) TestHasPermission() {
	u := s.users[0]

	// no roles - no permission
	hasPermission := u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 1})
	s.False(hasPermission)

	// has a role with explicit permission to the resource
	s.NoError(u.AddRole(s.T().Context(), "r1p1"))
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 1})
	s.True(hasPermission)
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 0})
	s.True(hasPermission)

	// role with insufficient permission but the right resource
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource1", Permission: "permission", RequiredLevel: 2})
	s.False(hasPermission)

	// role with a parent scope
	s.NoError(u.AddRole(s.T().Context(), "r12p1"))
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource2", Permission: "permission", RequiredLevel: 1})
	s.True(hasPermission)

	// role with no permission to the specified resource
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource4", Permission: "permission", RequiredLevel: 1})
	s.False(hasPermission)

	// permission to everything
	s.NoError(u.RemoveRole(s.T().Context(), "r1p1"))
	s.NoError(u.RemoveRole(s.T().Context(), "r12p1"))
	s.NoError(u.AddRole(s.T().Context(), "r1234p2"))
	hasPermission = u.HasPermission(gimlet.PermissionOpts{Resource: "resource4", Permission: "permission", RequiredLevel: 2})
	s.True(hasPermission)
}

func (s *UserTestSuite) TestHasDistroCreatePermission() {
	roleManager := s.env.RoleManager()

	usr := DBUser{
		Id: "basic_user",
	}
	s.NoError(usr.Insert(s.T().Context()))
	s.Require().False(usr.HasDistroCreatePermission())

	createRole := gimlet.Role{
		ID:          "create_distro",
		Name:        "create_distro",
		Scope:       "superuser_scope",
		Permissions: map[string]int{evergreen.PermissionDistroCreate: evergreen.DistroCreate.Value},
	}
	s.Require().NoError(roleManager.UpdateRole(createRole))
	s.Require().NoError(usr.AddRole(s.T().Context(), createRole.ID))

	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{evergreen.SuperUserPermissionsID},
	}
	s.Require().NoError(roleManager.AddScope(superUserScope))

	s.True(usr.HasDistroCreatePermission())
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

	users, err := FindNeedsReauthorization(s.T().Context(), 0)
	s.NoError(err)
	s.Require().Len(users, 4)
	s.True(containsUsers(users, "Test1", "Test2", "Test4", "Test5"), "should find all logged in users")
	s.False(containsUsers(users, "Test3"), "should not find logged out users")

	users, err = FindNeedsReauthorization(s.T().Context(), 0)
	s.NoError(err)
	s.Require().Len(users, 4)
	s.True(containsUsers(users, "Test1", "Test2", "Test4", "Test5"), "should find logged in users who have not exceeded max reauth attempts")
	s.False(containsUsers(users, "Test3", "Test6"), "should not find logged out users or users who have exceeded max reauth attempts")

	users, err = FindNeedsReauthorization(s.T().Context(), 30*time.Minute)
	s.NoError(err)
	s.Require().Len(users, 1)
	s.True(containsUsers(users, "Test2"), "should find logged in users who have exceeded the reauth limit")

	users, err = FindNeedsReauthorization(s.T().Context(), 24*time.Hour)
	s.NoError(err)
	s.Empty(users, "should not find users who have not exceeded the reauth limit")
}

func TestServiceUserOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	u := DBUser{
		Id:           "u",
		DispName:     "service_user",
		SystemRoles:  []string{"one"},
		EmailAddress: "myemail@mailplace.com",
	}
	assert.EqualError(t, AddOrUpdateServiceUser(t.Context(), u), "cannot update a non-service user")
	u.OnlyAPI = true
	assert.NoError(t, AddOrUpdateServiceUser(t.Context(), u))
	dbUser, err := FindOneByIdContext(t.Context(), u.Id)
	assert.NoError(t, err)
	assert.True(t, dbUser.OnlyAPI)
	assert.Equal(t, u.DispName, dbUser.DispName)
	assert.Equal(t, u.SystemRoles, dbUser.SystemRoles)
	u.APIKey = dbUser.APIKey
	assert.NotEmpty(t, u.APIKey)

	u.DispName = "another"
	u.SystemRoles = []string{"one", "two"}
	assert.NoError(t, AddOrUpdateServiceUser(t.Context(), u))
	dbUser, err = FindOneByIdContext(t.Context(), u.Id)
	assert.NoError(t, err)
	assert.True(t, dbUser.OnlyAPI)
	assert.Equal(t, u.DispName, dbUser.DispName)
	assert.Equal(t, u.SystemRoles, dbUser.SystemRoles)
	assert.Equal(t, u.APIKey, dbUser.APIKey)
	assert.Equal(t, u.EmailAddress, dbUser.EmailAddress)

	users, err := FindServiceUsers(t.Context())
	assert.NoError(t, err)
	assert.Len(t, users, 1)

	err = DeleteServiceUser(ctx, "doesntexist")
	assert.EqualError(t, err, "service user 'doesntexist' not found")
	err = DeleteServiceUser(ctx, u.Id)
	assert.NoError(t, err)
	dbUser, err = FindOneByIdContext(t.Context(), u.Id)
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

			dbUser, err := FindOneByIdContext(t.Context(), id)
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
	}

	assert.ElementsMatch(t, []string{
		"BuildBreakID",
		"PatchFinishID",
		"PatchFirstFailureID",
		"SpawnHostExpirationID",
		"SpawnHostOutcomeID",
	}, u.GeneralSubscriptionIDs())
}

func TestViewableProject(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()

	assert.NoError(t, db.ClearCollections(evergreen.RoleCollection, evergreen.ScopeCollection, Collection))

	restrictedProjectScope := gimlet.Scope{
		ID:        "restricted_project_scope",
		Name:      "restricted projects",
		Resources: []string{"parsley"},
		Type:      evergreen.ProjectResourceType,
	}
	err := rm.AddScope(restrictedProjectScope)
	assert.NoError(t, err)

	parsleyAccessRoleId := "parsley_access"
	parsleyAccessRole := gimlet.Role{
		ID:          parsleyAccessRoleId,
		Name:        "parsley access",
		Scope:       "restricted_project_scope",
		Permissions: map[string]int{evergreen.PermissionTasks: 20, evergreen.PermissionPatches: 10, evergreen.PermissionLogs: 10, evergreen.PermissionAnnotations: 10},
	}
	err = rm.UpdateRole(parsleyAccessRole)
	assert.NoError(t, err)

	unrestrictedProjectScope := gimlet.Scope{
		ID:        evergreen.UnrestrictedProjectsScope,
		Name:      "unrestricted projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"mci", "spruce"},
	}
	err = rm.AddScope(unrestrictedProjectScope)
	assert.NoError(t, err)

	basicProjectAccessRole := gimlet.Role{
		ID:          evergreen.BasicProjectAccessRole,
		Name:        "basic access",
		Scope:       evergreen.UnrestrictedProjectsScope,
		Permissions: map[string]int{evergreen.PermissionTasks: 20, evergreen.PermissionPatches: 10, evergreen.PermissionLogs: 10, evergreen.PermissionAnnotations: 10},
	}
	err = rm.UpdateRole(basicProjectAccessRole)
	assert.NoError(t, err)

	myUser := DBUser{
		Id: "me",
	}
	assert.NoError(t, myUser.Insert(t.Context()))
	err = myUser.AddRole(t.Context(), evergreen.BasicProjectAccessRole)
	assert.NoError(t, err)

	// assert that viewable projects contains the edit projects and the view projects
	projects, err := myUser.GetViewableProjects(ctx)
	assert.NoError(t, err)
	assert.Len(t, projects, 2)
	assert.Contains(t, projects, "mci")
	assert.Contains(t, projects, "spruce")

	// assert that adding a role to the user allows them to view the restricted project they didn't have access to before
	err = myUser.AddRole(t.Context(), parsleyAccessRoleId)
	assert.NoError(t, err)

	projects, err = myUser.GetViewableProjects(ctx)
	assert.NoError(t, err)
	assert.Len(t, projects, 3)
	assert.Contains(t, projects, "mci")
	assert.Contains(t, projects, "spruce")
	assert.Contains(t, projects, "parsley")
}

func TestViewableProjectSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	rm := env.RoleManager()

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
	assert.NoError(t, myUser.Insert(t.Context()))
	assert.NoError(t, myUser.AddRole(t.Context(), viewRole.ID))
	assert.NoError(t, myUser.AddRole(t.Context(), editRole.ID))
	assert.NoError(t, myUser.AddRole(t.Context(), otherRole.ID))

	// assert that viewable projects contains the edit projects and the view projects
	projects, err := myUser.GetViewableProjectSettings(ctx)
	assert.NoError(t, err)
	assert.Len(t, projects, 3)
	assert.Contains(t, projects, "edit1")
	assert.Contains(t, projects, "edit2")
	assert.Contains(t, projects, "view1")
	assert.NotContains(t, projects, "other")
}

func TestUpdateParsleySettings(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	usr := DBUser{
		Id: "me",
	}
	require.NoError(t, usr.Insert(t.Context()))

	newSettings := parsley.Settings{
		SectionsEnabled: utility.FalsePtr(),
	}
	err := usr.UpdateParsleySettings(t.Context(), newSettings)
	require.NoError(t, err)
	assert.False(t, utility.FromBoolPtr(usr.ParsleySettings.SectionsEnabled))

	dbUser, err := FindOneByIdContext(t.Context(), usr.Id)
	require.NoError(t, err)
	require.NotNil(t, dbUser)
	assert.False(t, utility.FromBoolPtr(dbUser.ParsleySettings.SectionsEnabled))
}

func TestUpdateBetaFeatures(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))

	usr := DBUser{
		Id: "me",
	}
	require.NoError(t, usr.Insert(t.Context()))

	dbUser, err := FindOneByIdContext(t.Context(), usr.Id)
	require.NoError(t, err)
	require.NotNil(t, dbUser)
	assert.False(t, dbUser.BetaFeatures.ParsleyAIEnabled)

	newBetaFeatureSettings := evergreen.BetaFeatures{
		ParsleyAIEnabled: true,
	}
	err = usr.UpdateBetaFeatures(t.Context(), newBetaFeatureSettings)
	require.NoError(t, err)
	assert.True(t, usr.BetaFeatures.ParsleyAIEnabled)

	dbUser, err = FindOneByIdContext(t.Context(), usr.Id)
	require.NoError(t, err)
	require.NotNil(t, dbUser)
	assert.True(t, dbUser.BetaFeatures.ParsleyAIEnabled)
}

func (s *UserTestSuite) TestClearUser() {
	// Error on non-existent user.
	s.Error(ClearUser(s.T().Context(), "asdf"))

	u, err := FindOneByIdContext(s.T().Context(), s.users[0].Id)
	s.NoError(err)
	s.NotNil(u)
	s.NotEmpty(u.Settings)
	s.Equal("Test1", u.Id)
	s.True(u.Settings.UseSpruceOptions.SpruceV1)
	s.NoError(u.AddRole(s.T().Context(), "r1p1"))
	s.NotEmpty(u.SystemRoles)
	s.NotEmpty(u.APIKey)

	s.NoError(ClearUser(s.T().Context(), u.Id))

	// Sensitive settings and roles should now be empty.
	u, err = FindOneByIdContext(s.T().Context(), s.users[0].Id)
	s.NoError(err)
	s.NotNil(u)

	s.Empty(u.Settings.GithubUser)
	s.Empty(u.Settings.SlackUsername)
	s.Empty(u.Settings.SlackMemberId)
	s.Empty(u.Roles())
	s.Empty(u.LoginCache)
	s.Empty(u.APIKey)

	// User should have spruce UI enabled.
	s.True(u.Settings.UseSpruceOptions.SpruceV1)

	// Should enable for user that previously had it false
	u, err = FindOneByIdContext(s.T().Context(), s.users[1].Id)
	s.NoError(err)
	s.NotNil(u)
	s.False(u.Settings.UseSpruceOptions.SpruceV1)
	s.NoError(ClearUser(s.T().Context(), u.Id))
	u, err = FindOneByIdContext(s.T().Context(), s.users[1].Id)
	s.NoError(err)
	s.True(u.Settings.UseSpruceOptions.SpruceV1)
}
