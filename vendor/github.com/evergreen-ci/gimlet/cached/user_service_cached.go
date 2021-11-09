package cached

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/pkg/errors"
)

// cacheUserManager creates a thin wrapper around a user cache.
type cachedUserManager struct {
	cache usercache.Cache
}

// NewUserManager returns a user manager backed by a cache that manages users.
func NewUserManager(cache usercache.Cache) (gimlet.UserManager, error) {
	if cache == nil {
		return nil, errors.New("cache cannot be nil")
	}
	return &cachedUserManager{cache: cache}, nil
}

func (um *cachedUserManager) GetUserByToken(_ context.Context, token string) (gimlet.User, error) {
	user, _, err := um.cache.Get(token)
	if err != nil {
		return nil, errors.Wrap(err, "could not get cached user with token")
	}
	if user == nil {
		return nil, errors.New("user not found in cache with token")
	}
	return user, nil
}

func (um *cachedUserManager) ReauthorizeUser(user gimlet.User) error {
	return errors.New("cannot reauthorize users for cached user manager")
}

func (um *cachedUserManager) CreateUserToken(username, password string) (string, error) {
	return "", errors.New("cannot create user tokens for cached user manager")
}

func (um *cachedUserManager) GetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	return um.cache.GetOrCreate(user)
}

func (*cachedUserManager) GetLoginHandler(string) http.HandlerFunc   { return nil }
func (*cachedUserManager) GetLoginCallbackHandler() http.HandlerFunc { return nil }
func (*cachedUserManager) IsRedirect() bool                          { return false }

func (um *cachedUserManager) GetUserByID(id string) (gimlet.User, error) {
	user, valid, err := um.cache.Find(id)
	if err != nil {
		return nil, errors.Wrap(err, "could not get cached user with ID")
	}
	if user == nil {
		return nil, errors.New("user not found in cache with ID")
	}
	if !valid {
		return nil, errors.New("user is invalid")
	}
	return user, nil
}

func (um *cachedUserManager) ClearUser(user gimlet.User, all bool) error {
	return um.cache.Clear(user, all)
}

func (um *cachedUserManager) GetGroupsForUser(id string) ([]string, error) {
	return nil, errors.New("cannot get groups for cached user manager")
}
