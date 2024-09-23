package auth

import (
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/cached"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/pkg/errors"
)

// NewExternalUserManager returns a [gimlet.UserManager] that's a thin wrapper around a database backed
// user cache.
func NewExternalUserManager() (gimlet.UserManager, error) {
	cache, err := usercache.NewExternal(usercache.ExternalOptions{
		PutUserGetToken: func(gimlet.User) (string, error) { return "", errors.New("not implemented") },
		GetUserByToken:  func(token string) (gimlet.User, bool, error) { return nil, false, errors.New("not implemented") },
		ClearUserToken:  func(u gimlet.User, all bool) error { return errors.New("not implemented") },
		GetUserByID: func(id string) (gimlet.User, bool, error) {
			user, err := getUserByID(id)
			return user, true, err
		},
		GetOrCreateUser: getOrCreateUser,
	})
	if err != nil {
		return nil, errors.Wrap(err, "making external user cache")
	}
	um, err := cached.NewUserManager(cache)
	if err != nil {
		return nil, errors.Wrap(err, "constructing external user manager")
	}
	return um, nil
}
