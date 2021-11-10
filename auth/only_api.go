package auth

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/cached"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/pkg/errors"
)

// NewOnlyAPIUserManager creates a user manager for special users that can only
// make API requests. Users cannot be created and must come from the database.
func NewOnlyAPIUserManager() (gimlet.UserManager, error) {
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
			user, err := findOnlyAPIUser(id)
			if err != nil {
				return nil, false, errors.Errorf("failed to get API-only user")
			}
			return user, true, nil
		},
		GetOrCreateUser: func(u gimlet.User) (gimlet.User, error) {
			user, err := findOnlyAPIUser(u.Username())
			if err != nil {
				return nil, errors.Wrap(err, "failed to get API-only user and cannot create new one")
			}
			return user, nil
		},
	}
	cache, err := usercache.NewExternal(opts)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create user cache")
	}
	return cached.NewUserManager(cache)
}

// findOnlyAPIUser finds an API-only user by ID
func findOnlyAPIUser(id string) (*user.DBUser, error) {
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

	return dbUser, nil
}
