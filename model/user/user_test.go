package user

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

var userTestConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(userTestConfig.SessionFactory())
}

type UserTestSuite struct {
	suite.Suite
	users []*DBUser
}

func TestDBUser(t *testing.T) {
	s := &UserTestSuite{}
	suite.Run(t, s)
}

func (s *UserTestSuite) SetupTest() {
	db.ClearCollections(Collection)
	s.users = []*DBUser{
		&DBUser{
			Id:     "Test1",
			APIKey: "12345",
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
		},
	}

	for _, user := range s.users {
		s.NoError(user.Insert())
	}
}

func (s *UserTestSuite) TeardownTest() {
	db.ClearCollections(Collection)
}

func (s *UserTestSuite) TestAddKey() {
	s.NoError(s.users[0].AddPublicKey("key1", "ssh-mock 67890"))

	u, err := FindOne(ById(s.users[0].Id))
	s.NoError(err)
	s.NotNil(u)

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
