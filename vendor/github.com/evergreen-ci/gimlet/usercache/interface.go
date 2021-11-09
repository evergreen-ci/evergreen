package usercache

import (
	"github.com/evergreen-ci/gimlet"
)

// PutUserGetToken returns a new token. Updating the user's TTL should happen in
// this function.
type PutUserGetToken func(gimlet.User) (string, error)

// GetUserByToken is a function provided by the client to retrieve cached users
// by token.
// It returns an error if and only if there was an error retrieving the user
// from the cache.
// It returns (<user>, true, nil) if the user is present in the cache and is
// valid.
// It returns (<user>, false, nil) if the user is present in the cache but has
// expired.
// It returns (nil, false, nil) if the user is not present in the cache.
type GetUserByToken func(string) (u gimlet.User, valid bool, err error)

// ClearUserToken is a function provided by the client to remove users' tokens
// from cache. Passing true will ignore the user passed and clear all users.
type ClearUserToken func(u gimlet.User, all bool) error

// GetUserByID is a function provided by the client to get a user from persistent storage.
type GetUserByID func(string) (u gimlet.User, valid bool, err error)

// GetOrCreateUser is a function provided by the client to get a user from
// persistent storage, or if the user does not exist, to create and save it.
type GetOrCreateUser func(gimlet.User) (gimlet.User, error)

type Cache interface {
	Add(gimlet.User) error
	Put(gimlet.User) (string, error)
	Clear(gimlet.User, bool) error
	GetOrCreate(gimlet.User) (gimlet.User, error)
	Get(token string) (u gimlet.User, valid bool, err error)
	Find(id string) (u gimlet.User, valid bool, err error)
}
