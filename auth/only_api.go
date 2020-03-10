package auth

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
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
	validUsers := make([]gimlet.User, 0, len(config.Users))
	validIDs := make([]string, 0, len(config.Users))

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
					user.APIKeyKey:  u.Key,
					user.RolesKey:   u.Roles,
					user.OnlyAPIKey: true,
				},
				"$setOnInsert": bson.M{
					user.IdKey: u.Username,
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

		validUsers = append(validUsers, dbUser)
		validIDs = append(validIDs, u.Username)
	}

	// Remove API-only users that are no longer in the API-only config.
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	if _, err := env.DB().Collection(user.Collection).DeleteMany(ctx, bson.M{
		user.IdKey:      bson.M{"$nin": validIDs},
		user.OnlyAPIKey: true,
	}); err != nil {
		catcher.Wrap(err, "could not delete old API-only users from DB")
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
			user, err := findOnlyAPIUser(id, validIDs, validUsers)
			if err != nil {
				return nil, false, errors.Errorf("failed to get API-only user")
			}
			return user, true, nil
		},
		GetOrCreateUser: func(u gimlet.User) (gimlet.User, error) {
			user, err := findOnlyAPIUser(u.Username(), validIDs, validUsers)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get API-only user and cannot create new one")
			}
			return user, nil
		},
	}
	cache, err := usercache.NewExternal(opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not create DB cache")
	}
	return cached.NewUserManager(cache)
}

// findOnlyAPIUser finds an API-only user by ID and verifies that it is a valid
// user against the list of authoritative valid users.
func findOnlyAPIUser(id string, validIDs []string, validUsers []gimlet.User) (*user.DBUser, error) {
	if !util.StringSliceContains(validIDs, id) {
		return nil, errors.Errorf("user '%s' does not match a valid API-only user", validIDs)
	}
	dbUser, err := user.FindOne(db.Query(bson.M{
		user.IdKey:      id,
		user.OnlyAPIKey: true,
	}))
	if err != nil {
		return nil, errors.Wrap(err, "could not find API-only user in DB")
	}
	if dbUser == nil {
		return nil, errors.Errorf("no such user '%s' in DB", id)
	}
	for _, user := range validUsers {
		if user.Username() == dbUser.Username() && user.GetAPIKey() == dbUser.GetAPIKey() {
			return dbUser, nil
		}
	}
	return nil, errors.Errorf("user '%s' found but not in list of valid API-only users", id)
}
