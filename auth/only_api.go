package auth

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/cached"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"gopkg.in/mgo.v2/bson"

	"github.com/pkg/errors"
)

// NewOnlyAPIUserManager creates a user manager for special users that can only
// make API requests. Users cannot be created and must come from the database.
func NewOnlyAPIUserManager(config *evergreen.OnlyAPIAuthConfig) (gimlet.UserManager, error) {
	users := make([]gimlet.User, 0, len(config.Users))
	ids := make([]string, 0, len(config.Users))

	catcher := grip.NewBasicCatcher()
	for _, u := range config.Users {
		// Synchronize API-only users with those in the database.
		checkUser, err := user.FindOneById(u.Username)
		if err != nil {
			catcher.Wrapf(err, "could not check whether user '%s' exists in the DB already", u.Username)
			continue
		}
		if checkUser != nil && !checkUser.OnlyAPI {
			catcher.Errorf("cannot use API-only user '%s' who is not marked as API-only", u.Username)
			continue
		}
		_, err = db.Upsert(user.Collection,
			bson.M{user.IdKey: u.Username, user.OnlyAPIKey: true},
			bson.M{
				"$set": bson.M{
					user.APIKeyKey: u.Key,
					user.RolesKey:  u.Roles,
				},
				"$setOnInsert": bson.M{
					user.IdKey:      u.Username,
					user.OnlyAPIKey: true,
				},
			},
		)
		if err != nil {
			catcher.Wrapf(err, "could not upsert API-only user '%s'", u.Username)
			continue
		}

		dbUser := &user.DBUser{
			Id:          u.Username,
			APIKey:      u.Key,
			SystemRoles: u.Roles,
			OnlyAPI:     true,
		}

		users = append(users, dbUser)
		ids = append(ids, u.Username)
	}

	// Remove API-only users that are no longer in the API-only config.
	if err := db.RemoveAll(user.Collection, bson.M{user.IdKey: bson.M{"$nin": ids}, user.OnlyAPIKey: true}); err != nil {
		catcher.Wrap(err, "failed to remove nonexistent API-only users from database")
	}

	if catcher.HasErrors() {
		grip.Critical(message.WrapError(catcher.Resolve(), message.Fields{
			"message": "failed to create API-only manager",
			"reason":  "problems updating API-only users in database",
		}))
		return nil, errors.Wrap(catcher.Resolve(), "could not initialize API-only user manager")
	}

	opts := usercache.ExternalOptions{
		PutUserGetToken: func(gimlet.User) (string, error) {
			return "", errors.New("cannot put new users in DB")
		},
		GetUserByToken: func(string) (gimlet.User, bool, error) {
			return nil, false, errors.New("cannot get user by login token")
		},
		ClearUserToken: func(gimlet.User, bool) error {
			return errors.New("cannot clear user token")
		},
		GetUserByID: func(id string) (gimlet.User, bool, error) {
			dbUser, err := user.FindOneById(id)
			if err != nil {
				return nil, false, errors.Wrapf(err, "could not find user '%s' in DB", id)
			}
			for _, user := range users {
				if user.Username() == dbUser.Username() && user.GetAPIKey() == dbUser.GetAPIKey() {
					return dbUser, true, nil
				}
			}
			return nil, false, errors.Errorf("user '%s' found in DB but does not match a pre-populated API-only user", id)
		},
		GetOrCreateUser: func(u gimlet.User) (gimlet.User, error) {
			dbUser, err := user.FindOneById(u.Username())
			if err != nil {
				return nil, errors.Wrapf(err, "could not find user '%s' in DB", u.Username())
			}
			if dbUser == nil {
				return nil, errors.Errorf("no such user '%s' in db", u.Username())
			}
			for _, user := range users {
				if user.Username() == dbUser.Username() && user.GetAPIKey() == dbUser.GetAPIKey() {
					return dbUser, nil
				}
			}
			return nil, errors.New("cannot create user that does not already exist")
		},
	}
	cache, err := usercache.NewExternal(opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not create DB cache")
	}
	return cached.NewUserManager(cache)
}
