package auth

import (
	"crypto/md5"
	"fmt"
	"github.com/evergreen-ci/evergreen"
)

// NaiveUserManager implements the UserManager interface and has a list of AuthUsers{UserName, DisplayName, Password, Email string}
// which is stored in the settings configuration file.
// Note: This use of the UserManager is recommended for dev/test purposes only and users who need high security authentication
// mechanisms should rely on a different authentication mechanism.
type NaiveUserManager struct {
	users []*evergreen.AuthUser
}

func NewNaiveUserManager(naiveAuthConfig *evergreen.NaiveAuthConfig) (*NaiveUserManager, error) {
	users := naiveAuthConfig.Users
	return &NaiveUserManager{users}, nil
}

// GetUserByToken does a find by creating a temporary token from the index of the user on the list,
// the email of the user and a hash of the username and password, checking it against the token string
// and returning a User if there is a match.
func (b *NaiveUserManager) GetUserByToken(token string) (User, error) {
	for i, user := range b.users {
		//check to see if token exists
		possibleToken := fmt.Sprintf("%v:%v:%v", i, user.Email, md5.Sum([]byte(user.Username+user.Password)))
		if token == possibleToken {
			return &simpleUser{
				user.Username,
				user.DisplayName,
				user.Email,
			}, nil
		}
	}
	return nil, fmt.Errorf("No valid user found")
}

// CreateUserToken finds the user with the same username and password in its list of users and creates a token
// that is a combination of the index of the list the user is at, the email address and a hash of the username
// and password and returns that token.
func (b *NaiveUserManager) CreateUserToken(username, password string) (string, error) {
	for i, user := range b.users {
		if user.Username == username && user.Password == password {
			// return a token that is a hash of the index, user's email and username and password hashed.
			return fmt.Sprintf("%v:%v:%v", i, user.Email, md5.Sum([]byte(user.Username+user.Password))), nil
		}
	}
	return "", fmt.Errorf("No valid user for the given username and password")
}
