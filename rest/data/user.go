package data

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/pkg/errors"
)

// DBUserConnector is a struct that implements the User related interface
// of the Connector interface through interactions with the backing database.
type DBUserConnector struct{}

// FindUserById uses the service layer's user type to query the backing database for
// the user with the given Id.
func (tc *DBUserConnector) FindUserById(userId string) (auth.APIUser, error) {
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
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "expected a Slack username, but got a channel",
		}
	}
	settings.SlackUsername = strings.TrimPrefix(settings.SlackUsername, "@")

	var sub event.Subscriber
	switch settings.Notifications.PatchFinish {
	case user.PreferenceSlack:
		sub = event.NewSlackSubscriber(fmt.Sprintf("#%s", settings.SlackUsername))

	case user.PreferenceEmail:
		sub = event.NewEmailSubscriber(dbUser.Email())
	}

	s, err := event.FindSelfSubscriptionForUsersPatches(dbUser.Id)
	if err != nil {
		return errors.Wrap(err, "failed to fetch subscription")
	}
	if s == nil {
		sub := event.NewSelfSubscriptionForUsersPatches(dbUser.Id, sub)
		err = sub.Upsert()

	} else {
		s.Subscriber = sub
		if err = s.Validate(); err != nil {
			err = event.RemoveSubscription(s.ID)

		} else {
			err = s.Upsert()
		}
	}

	if err != nil {
		return errors.Wrap(err, "failed to update patch subscription")
	}

	return model.SaveUserSettings(dbUser.Id, settings)
}

// MockUserConnector stores a cached set of users that are queried against by the
// implementations of the UserConnector interface's functions.
type MockUserConnector struct {
	CachedUsers map[string]*user.DBUser
}

// FindUserById provides a mock implementation of the User functions
// from the Connector that does not need to use a database.
// It returns results based on the cached users in the MockUserConnector.
func (muc *MockUserConnector) FindUserById(userId string) (auth.APIUser, error) {
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
