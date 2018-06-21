package data

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/gimlet"
	"github.com/k0kubun/pp"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "expected a Slack username, but got a channel",
		}
	}
	settings.SlackUsername = strings.TrimPrefix(settings.SlackUsername, "@")
	settings.Notifications.PatchFinishID = dbUser.Settings.Notifications.PatchFinishID
	settings.Notifications.SpawnHostID = dbUser.Settings.Notifications.SpawnHostID

	var subscriber event.Subscriber
	switch settings.Notifications.PatchFinish {
	case user.PreferenceSlack:
		subscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))

	case user.PreferenceEmail:
		subscriber = event.NewEmailSubscriber(dbUser.Email())
	}

	var err error
	settings.Notifications.PatchFinishID, err = updateSubscription(dbUser.Id, dbUser.Settings.Notifications.PatchFinishID, subscriber, event.NewPatchOutcomeSubscriptionByOwner)
	if err != nil {
		return err
	}

	switch settings.Notifications.SpawnHost {
	case user.PreferenceSlack:
		subscriber = event.NewSlackSubscriber(fmt.Sprintf("@%s", settings.SlackUsername))

	case user.PreferenceEmail:
		subscriber = event.NewEmailSubscriber(dbUser.Email())

	default:
		subscriber = event.Subscriber{}
	}

	settings.Notifications.SpawnHostID, err = updateSubscription(dbUser.Id, dbUser.Settings.Notifications.SpawnHostID, subscriber, event.NewSpawnHostOutcomeByOwner)
	if err != nil {
		return err
	}

	return model.SaveUserSettings(dbUser.Id, settings)
}

func updateSubscription(user string, id bson.ObjectId, sub event.Subscriber, f func(string, event.Subscriber) event.Subscription) (bson.ObjectId, error) {
	var subscription *event.Subscription
	outID := id
	if id.Valid() {
		var err error
		subscription, err = event.FindSubscriptionByID(id)
		if err != nil {
			return id, err
		}
		// in the event the database has bad data, we proceed as if
		// a new subscription is being created.
	}

	if sub.Validate() == nil {
		if subscription == nil {
			temp := f(user, sub)
			subscription = &temp
			outID = subscription.ID

		} else {
			subscription.Subscriber = sub
		}

		subscription.OwnerType = event.OwnerTypePerson
		subscription.Owner = user

		if err := subscription.Upsert(); err != nil {
			return outID, errors.Wrap(err, "failed to update subscription")
		}
		outID = subscription.ID

	} else {
		pp.Println("try out", outID.Hex(), id.Hex())
		if id.Valid() {
			if err := event.RemoveSubscription(outID); err != nil {
				return outID, err
			}
			outID = ""
		}
	}

	return outID, nil
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
