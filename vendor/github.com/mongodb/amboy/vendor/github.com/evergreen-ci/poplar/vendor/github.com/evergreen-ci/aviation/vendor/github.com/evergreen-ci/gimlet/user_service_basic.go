package gimlet

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// BasicUserManager implements the UserManager interface and has a list of
// BasicUsers which is passed into the constructor.
type BasicUserManager struct {
	users []basicUser
}

// NewBasicUserManager is a constructor to create a BasicUserManager from
// a list of basic users. It requires a user created by NewBasicUser.
func NewBasicUserManager(users []User) (UserManager, error) {
	catcher := grip.NewBasicCatcher()
	basicUsers := []basicUser{}
	var bu *basicUser
	var ok bool
	for _, u := range users {
		if bu, ok = u.(*basicUser); !ok {
			catcher.Errorf("%T is not a basicUser", u)
			continue
		}
		basicUsers = append(basicUsers, *bu)
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return &BasicUserManager{basicUsers}, nil
}

// GetUserByToken does a find by creating a temporary token from the index of
// the user on the list, the email of the user and a hash of the username and
// password, checking it against the token string and returning a User if
// there is a match.
func (um *BasicUserManager) GetUserByToken(_ context.Context, token string) (User, error) {
	for i, user := range um.users {
		//check to see if token exists
		possibleToken := fmt.Sprintf("%v:%v:%v", i, user.EmailAddress, md5.Sum([]byte(user.ID+user.Password)))
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
			return fmt.Sprintf("%v:%v:%v", i, user.EmailAddress, md5.Sum([]byte(user.ID+user.Password))), nil
		}
	}
	return "", errors.New("No valid user for the given username and password")
}

func (*BasicUserManager) GetLoginHandler(string) http.HandlerFunc   { return nil }
func (*BasicUserManager) GetLoginCallbackHandler() http.HandlerFunc { return nil }
func (*BasicUserManager) IsRedirect() bool                          { return false }

func (um *BasicUserManager) GetUserByID(id string) (User, error) {
	for _, user := range um.users {
		if user.ID == id {
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

	newUser := basicUser{
		ID:           u.Username(),
		EmailAddress: u.Email(),
	}
	um.users = append(um.users, newUser)
	return &newUser, nil
}

func (b *BasicUserManager) ClearUser(u User, all bool) error {
	return errors.New("Naive Authentication does not support Clear User")
}
