package data

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/suite"
)

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

type DBUserConnectorSuite struct {
	suite.Suite
	sc       *DBConnector
	numUsers int
	users    []*user.DBUser
}

func (s *DBUserConnectorSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection, event.SubscriptionsCollection))
	s.sc = &DBConnector{}
	s.numUsers = 10

	for i := 0; i < s.numUsers; i++ {
		uid := fmt.Sprintf("user_%d", i)
		testUser := &user.DBUser{
			Id:           uid,
			APIKey:       fmt.Sprintf("apikey_%d", i),
			EmailAddress: fmt.Sprintf("%s@domain.invalid", uid),
			PubKeys: []user.PubKey{
				{
					Name: fmt.Sprintf("user_%d_0", i),
					Key:  "ssh-rsa 12345",
				},
			},
		}
		s.NoError(testUser.Insert())
		s.users = append(s.users, testUser)
	}
}

func (s *DBUserConnectorSuite) TestFindUserById() {
	for i := 0; i < s.numUsers; i++ {
		found, err := s.sc.FindUserById(fmt.Sprintf("user_%d", i))
		s.NoError(err)
		s.Equal(found.GetAPIKey(), fmt.Sprintf("apikey_%d", i))
	}

	found, err := s.sc.FindUserById("fake_user")
	s.Nil(found)
	s.NoError(err)
}

func (s *DBUserConnectorSuite) TestDeletePublicKey() {
	for _, u := range s.users {
		s.NoError(s.sc.DeletePublicKey(u, u.Id+"_0"))

		dbUser, err := user.FindOne(user.ById(u.Id))
		s.NoError(err)
		s.Len(dbUser.PubKeys, 0)
	}
}

func (s *DBUserConnectorSuite) TestUpdateSettings() {
	settings := user.UserSettings{
		SlackUsername: "@test",
		Notifications: user.NotificationPreferences{
			BuildBreak:  user.PreferenceEmail,
			PatchFinish: user.PreferenceSlack,
		},
	}
	settings.Notifications.PatchFinish = ""
	s.NoError(s.sc.UpdateSettings(s.users[0], settings))
	sub, err := event.FindSelfSubscriptionForUsersPatches(s.users[0].Id)
	s.NoError(err)
	s.Nil(sub)

	settings.Notifications.PatchFinish = user.PreferenceSlack
	s.NoError(s.sc.UpdateSettings(s.users[0], settings))
	sub, err = event.FindSelfSubscriptionForUsersPatches(s.users[0].Id)
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.SlackSubscriberType, sub.Subscriber.Type)

	// should modify the existing subscription
	settings.Notifications.PatchFinish = user.PreferenceEmail
	s.NoError(s.sc.UpdateSettings(s.users[0], settings))
	sub, err = event.FindSelfSubscriptionForUsersPatches(s.users[0].Id)
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.EmailSubscriberType, sub.Subscriber.Type)

	// should delete the existing subscription
	settings.Notifications.PatchFinish = ""
	s.NoError(s.sc.UpdateSettings(s.users[0], settings))
	sub, err = event.FindSelfSubscriptionForUsersPatches(s.users[0].Id)
	s.NoError(err)
	s.Nil(sub)

	settings.SlackUsername = "#Test"
	s.EqualError(s.sc.UpdateSettings(s.users[0], settings), "expected a Slack username, but got a channel")
}

func TestDBUserConnector(t *testing.T) {
	s := &DBUserConnectorSuite{}
	suite.Run(t, s)
}
