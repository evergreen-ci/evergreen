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

}

type DBUserConnectorSuite struct {
	suite.Suite
	numUsers int
	users    []*user.DBUser
}

func (s *DBUserConnectorSuite) SetupTest() {
	s.NoError(db.ClearCollections(user.Collection, event.SubscriptionsCollection))
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

func (s *DBUserConnectorSuite) getNotificationSettings(index int) *user.NotificationPreferences {
	found, err := user.FindOneById(s.users[index].Id)
	s.NoError(err)
	s.Require().NotNil(found)

	s.users[index].Settings = found.Settings

	return &found.Settings.Notifications
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

	s.NoError(UpdateSettings(s.users[0], settings))
	pref := s.getNotificationSettings(0)
	s.NotNil(pref)
	s.Equal("", pref.PatchFinishID)

	// Should create a new subscription
	settings.Notifications.PatchFinish = user.PreferenceSlack
	s.NoError(UpdateSettings(s.users[0], settings))
	pref = s.getNotificationSettings(0)
	s.NotEqual("", pref.PatchFinishID)
	sub, err := event.FindSubscriptionByID(pref.PatchFinishID)
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.SlackSubscriberType, sub.Subscriber.Type)
	settings.Notifications = *pref

	// should modify the existing subscription
	settings.Notifications.PatchFinish = user.PreferenceEmail
	s.NoError(UpdateSettings(s.users[0], settings))
	pref = s.getNotificationSettings(0)
	s.NotNil(pref)
	s.NotEqual("", pref.PatchFinishID)
	sub, err = event.FindSubscriptionByID(pref.PatchFinishID)
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.EmailSubscriberType, sub.Subscriber.Type)
	settings.Notifications = *pref

	// should delete the existing subscription
	settings.Notifications.PatchFinish = ""
	s.NoError(UpdateSettings(s.users[0], settings))
	pref = s.getNotificationSettings(0)
	s.NotNil(pref)
	s.Equal("", pref.PatchFinishID)
	settings.Notifications = *pref

	settings.SlackUsername = "#Test"
	s.EqualError(UpdateSettings(s.users[0], settings), "400 (Bad Request): expected a Slack username, but got a channel")
}

func (s *DBUserConnectorSuite) TestUpdateSettingsCommitQueue() {
	settings := user.UserSettings{
		SlackUsername: "@test",
		Notifications: user.NotificationPreferences{
			CommitQueue: user.PreferenceSlack,
		},
	}

	// Should create a new subscription
	s.NoError(UpdateSettings(s.users[0], settings))
	pref := s.getNotificationSettings(0)
	s.NotEqual("", pref.CommitQueueID)
	sub, err := event.FindSubscriptionByID(pref.CommitQueueID)
	s.NoError(err)
	s.Require().NotNil(sub)
	s.Equal(event.SlackSubscriberType, sub.Subscriber.Type)
}

func TestDBUserConnector(t *testing.T) {
	s := &DBUserConnectorSuite{}
	suite.Run(t, s)
}
