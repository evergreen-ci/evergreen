package gimlet

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
)

// BasicUserManager implements the UserManager interface and has a list of
// BasicUsers which is passed into the constructor.
type BasicUserManager struct {
	users []BasicUser
	rm    RoleManager
}

// NewBasicUserManager is a constructor to create a BasicUserManager from
// a list of basic users. It requires a user created by NewBasicUser.
func NewBasicUserManager(users []BasicUser, rm RoleManager) (UserManager, error) {
	return &BasicUserManager{users: users, rm: rm}, nil
}

// GetUserByToken does a find by creating a temporary token from the index of
// the user on the list, the email of the user and a hash of the username and
// password, checking it against the token string and returning a User if
// there is a match.
func (um *BasicUserManager) GetUserByToken(_ context.Context, token string) (User, error) {
	for i, user := range um.users {
		// Check to see if the token exists.
		possibleToken := makeToken(user, i)
		if token == possibleToken {
			return &user, nil
		}
	}
	return nil, errors.New("No valid user found")
}

// CreateUserToken finds the user with the same username and password in its
// list of users and creates a token that is a combination of the index of the
// list the user is at, the email address and a hash of the username and
// password and returns that token.
func (um *BasicUserManager) CreateUserToken(username, password string) (string, error) {
	for i, user := range um.users {
		if user.ID == username && user.Password == password {
			// return a token that is a hash of the index, user's email and username and password hashed.
			return makeToken(user, i), nil
		}
	}
	return "", errors.New("No valid user for the given username and password")
}

func (*BasicUserManager) GetLoginHandler(string) http.HandlerFunc   { return nil }
func (*BasicUserManager) GetLoginCallbackHandler() http.HandlerFunc { return nil }
func (*BasicUserManager) IsRedirect() bool                          { return false }
func (um *BasicUserManager) ReauthorizeUser(user User) error {
	for _, u := range um.users {
		if user.Username() == u.Username() {
			return nil
		}
	}
	return errors.Errorf("user '%s 'not found", user.Username())
}

func (um *BasicUserManager) isInvalid(username string) bool {
	for _, user := range um.users {
		if user.ID == username {
			return user.invalid
		}
	}

	return true
}

func (um *BasicUserManager) setInvalid(username string, invalid bool) {
	for i := range um.users {
		if um.users[i].ID == username {
			um.users[i].invalid = invalid
			return
		}
	}
}

func (um *BasicUserManager) GetUserByID(id string) (User, error) {
	for _, user := range um.users {
		if user.ID == id {
			if user.invalid {
				return nil, errors.Errorf("user %s not authorized!", id)
			}
			return &user, nil
		}
	}
	return nil, errors.Errorf("user %s not found!", id)
}

func (um *BasicUserManager) GetOrCreateUser(u User) (User, error) {
	existingUser, err := um.GetUserByID(u.Username())
	if err == nil {
		return existingUser, nil
	}

	newUser := &BasicUser{
		ID:           u.Username(),
		EmailAddress: u.Email(),
		AccessRoles:  u.Roles(),
		roleManager:  um.rm,
	}
	um.users = append(um.users, *newUser)
	return newUser, nil
}

func (b *BasicUserManager) ClearUser(u User, all bool) error {
	return errors.New("Naive Authentication does not support Clear User")
}

func (b *BasicUserManager) GetGroupsForUser(userId string) ([]string, error) {
	for _, user := range b.users {
		if user.ID == userId {
			return user.AccessRoles, nil
		}
	}

	return nil, errors.Errorf("user %s not found", userId)
}

// makeToken generates a token for a user.
func makeToken(u BasicUser, index int) string {
	return fmt.Sprintf("%v:%v:%v", index, u.Email(), md5.Sum([]byte(u.Username()+u.Password)))
}
