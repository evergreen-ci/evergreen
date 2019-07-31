package user

import (
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
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

func (s *UserTestSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection))
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
				Token: "1234",
				TTL:   time.Now(),
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
	}

	for _, user := range s.users {
		s.NoError(user.Insert())
	}
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

	u2, err := FindOneById(s.users[1].Id)
	s.NoError(err)
	s.Equal(s.users[1].Id, u2.Id)

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

	// Fresh user with no token should generate new token
	token5, err := PutLoginCache(s.users[2])
	s.NoError(err)
	u5, err := FindOneById(s.users[2].Id)
	s.Equal(token5, u5.LoginCache.Token)
	s.NoError(err)
	s.NotEmpty(token5)
	s.NotEqual(token1, token5)
	s.NotEqual(token2, token5)
	s.NotEqual(token3, token5)
	s.NotEqual(token4, token5)
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
