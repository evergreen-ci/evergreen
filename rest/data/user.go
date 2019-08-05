package data

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/feedback"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBUserConnector is a struct that implements the User related interface
// of the Connector interface through interactions with the backing database.
type DBUserConnector struct{}

// FindUserById uses the service layer's user type to query the backing database for
// the user with the given Id.
func (tc *DBUserConnector) FindUserById(userId string) (gimlet.User, error) {
	t, err := user.FindOne(user.ById(userId))
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (u *DBUserConnector) AddPublicKey(user *user.DBUser, keyName, keyValue string) error {
	return user.AddPublicKey(keyName, keyValue)
}

func (u *DBUserConnector) DeletePublicKey(user *user.DBUser, keyName string) error {
	return user.DeletePublicKey(keyName)
}

func (u *DBUserConnector) UpdateSettings(dbUser *user.DBUser, settings user.UserSettings) error {
	if strings.HasPrefix(settings.SlackUsername, "#") {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "expected a Slack username, but got a channel",
		}
	}
	settings.SlackUsername = strings.TrimPrefix(settings.SlackUsername, "@")
	settings.Notifications.PatchFinishID = dbUser.Settings.Notifications.PatchFinishID
	settings.Notifications.SpawnHostOutcomeID = dbUser.Settings.Notifications.SpawnHostOutcomeID
	settings.Notifications.SpawnHostExpirationID = dbUser.Settings.Notifications.SpawnHostExpirationID
	settings.Notifications.CommitQueueID = dbUser.Settings.Notifications.CommitQueueID

	var patchSubscriber event.Subscriber
	switch settings.Notifications.PatchFinish {
	case user.PreferenceSlack:
		patchSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		patchSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	patchSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionPatchOutcome,
		dbUser.Settings.Notifications.PatchFinishID, patchSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if patchSubscription != nil {
		settings.Notifications.PatchFinishID = patchSubscription.ID
	} else {
		settings.Notifications.PatchFinishID = ""
	}

	var buildBreakSubscriber event.Subscriber
	switch settings.Notifications.BuildBreak {
	case user.PreferenceSlack:
		buildBreakSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		buildBreakSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	buildBreakSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionBuildBreak,
		dbUser.Settings.Notifications.BuildBreakID, buildBreakSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if buildBreakSubscription != nil {
		settings.Notifications.BuildBreakID = buildBreakSubscription.ID
	} else {
		settings.Notifications.BuildBreakID = ""
	}

	var spawnhostSubscriber event.Subscriber
	switch settings.Notifications.SpawnHostExpiration {
	case user.PreferenceSlack:
		spawnhostSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		spawnhostSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	spawnhostSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionSpawnhostExpiration,
		dbUser.Settings.Notifications.SpawnHostExpirationID, spawnhostSubscriber, dbUser.Id)
	if err != nil {
		return err
	}
	if spawnhostSubscription != nil {
		settings.Notifications.SpawnHostExpirationID = spawnhostSubscription.ID
	} else {
		settings.Notifications.SpawnHostExpirationID = ""
	}

	var spawnHostOutcomeSubscriber event.Subscriber
	switch settings.Notifications.SpawnHostOutcome {
	case user.PreferenceSlack:
		spawnHostOutcomeSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		spawnHostOutcomeSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	spawnHostOutcomeSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionSpawnHostOutcome,
		dbUser.Settings.Notifications.SpawnHostOutcomeID, spawnHostOutcomeSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "failed to create spawn host outcome subscription")
	}
	if spawnHostOutcomeSubscription != nil {
		settings.Notifications.SpawnHostOutcomeID = spawnHostOutcomeSubscription.ID
	} else {
		settings.Notifications.SpawnHostOutcomeID = ""
	}

	var commitQueueSubscriber event.Subscriber
	switch settings.Notifications.CommitQueue {
	case user.PreferenceSlack:
		commitQueueSubscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))
	case user.PreferenceEmail:
		commitQueueSubscriber = event.NewEmailSubscriber(dbUser.Email())
	}
	commitQueueSubscription, err := event.CreateOrUpdateImplicitSubscription(event.ImplicitSubscriptionCommitQueue,
		dbUser.Settings.Notifications.CommitQueueID, commitQueueSubscriber, dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "failed to create commit queue subscription")
	}
	if commitQueueSubscription != nil {
		settings.Notifications.CommitQueueID = commitQueueSubscription.ID
	} else {
		settings.Notifications.CommitQueueID = ""
	}

	return model.SaveUserSettings(dbUser.Id, settings)
}

func (u *DBUserConnector) SubmitFeedback(feedback feedback.FeedbackSubmission) error {
	return errors.Wrap(feedback.Insert(), "error saving feedback")
}

// MockUserConnector stores a cached set of users that are queried against by the
// implementations of the UserConnector interface's functions.
type MockUserConnector struct {
	CachedUsers map[string]*user.DBUser
}

// FindUserById provides a mock implementation of the User functions
// from the Connector that does not need to use a database.
// It returns results based on the cached users in the MockUserConnector.
func (muc *MockUserConnector) FindUserById(userId string) (gimlet.User, error) {
	u := muc.CachedUsers[userId]
	return u, nil
}

func (muc *MockUserConnector) AddPublicKey(dbuser *user.DBUser, keyName, keyValue string) error {
	u, ok := muc.CachedUsers[dbuser.Id]
	if !ok {
		return errors.New(fmt.Sprintf("User '%s' doesn't exist", dbuser.Id))
	}

	_, err := u.GetPublicKey(keyName)
	if err == nil {
		return errors.New(fmt.Sprintf("User '%s' already has a key '%s'", dbuser.Id, keyName))
	}

	u.PubKeys = append(u.PubKeys, user.PubKey{
		Name:      keyName,
		Key:       keyValue,
		CreatedAt: time.Now(),
	})

	return nil
}

func (muc *MockUserConnector) DeletePublicKey(u *user.DBUser, keyName string) error {
	cu, ok := muc.CachedUsers[u.Id]
	if !ok {
		return errors.New(fmt.Sprintf("User '%s' doesn't exist", u.Id))
	}

	newKeys := []user.PubKey{}
	var found bool = false
	for _, key := range cu.PubKeys {
		if key.Name != keyName {
			newKeys = append(newKeys, key)
		} else {
			found = true
		}
	}
	if !found {
		return errors.New(fmt.Sprintf("User '%s' has no key named '%s'", u.Id, keyName))
	}
	cu.PubKeys = newKeys
	return nil
}

func (muc *MockUserConnector) UpdateSettings(user *user.DBUser, settings user.UserSettings) error {
	return errors.New("UpdateSettings not implemented for mock connector")
}

func (u *MockUserConnector) SubmitFeedback(feedback feedback.FeedbackSubmission) error {
	return nil
}
