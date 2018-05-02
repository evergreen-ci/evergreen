package user

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
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
	s.NoError(s.users[0].AddPublicKey("key1", "ssh-mock 67890"))
	key, err := s.users[0].GetPublicKey("key1")
	s.Equal(key, "ssh-mock 67890")
	s.NoError(err)

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

func (s *UserTestSuite) TestBuildSubscriber() {
	u := &DBUser{
		Id:           "Test2",
		EmailAddress: "a@b.invalid",
		Settings: UserSettings{
			SlackUsername: "test2",
			Notifications: NotificationPreferences{
				BuildBreak:  PreferenceEmail,
				PatchFinish: PreferenceSlack,
			},
		},
	}
	sub, err := u.BuildBreakSubscriber()
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.EmailSubscriberType, sub.Type)

	sub, err = u.PatchFinishSubscriber()
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.SlackSubscriberType, sub.Type)

	u.Settings.Notifications.PatchFinish = "invalid"
	sub, err = u.PatchFinishSubscriber()
	s.EqualError(err, "unknown subscriber preference: invalid")

	u.Settings.Notifications.BuildBreak = "invalid"
	sub, err = u.BuildBreakSubscriber()
	s.EqualError(err, "unknown subscriber preference: invalid")

	u.Settings.Notifications.PatchFinish = ""
	sub, err = u.PatchFinishSubscriber()
	s.Nil(sub)
	s.NoError(err)

	u.Settings.Notifications.BuildBreak = ""
	sub, err = u.BuildBreakSubscriber()
	s.Nil(sub)
	s.NoError(err)
}
