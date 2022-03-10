package data

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
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

func (s *DBUserConnectorSuite) TestFindUserById() {
	for i := 0; i < s.numUsers; i++ {
		found, err := FindUserById(fmt.Sprintf("user_%d", i))
		s.NoError(err)
		s.Equal(found.GetAPIKey(), fmt.Sprintf("apikey_%d", i))
	}

	found, err := FindUserById("fake_user")
	s.Nil(found)
	s.NoError(err)
}

func (s *DBUserConnectorSuite) TestDeletePublicKey() {
	for _, u := range s.users {
		s.NoError(DeletePublicKey(u, u.Id+"_0"))

		dbUser, err := user.FindOne(user.ById(u.Id))
		s.NoError(err)
		s.Len(dbUser.PubKeys, 0)
	}
}

func (s *DBUserConnectorSuite) getNotificationSettings(index int) *user.NotificationPreferences {
	found, err := FindUserById(s.users[index].Id)
	s.NoError(err)
	s.Require().NotNil(found)
	user, ok := found.(*user.DBUser)
	s.Require().True(ok)

	s.users[index].Settings = user.Settings

	return &user.Settings.Notifications
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	suite.Run(t, s)
}
