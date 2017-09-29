package auth

import (
	"net/http"
)

// User describes an Evergreen user and is returned by a UserManager.
type User interface {
	DisplayName() string
	Email() string
	Username() string
	IsNil() bool
}

// User describes an Evergreen user that is going through the API.
type APIUser interface {
	User
	GetAPIKey() string
}

// UserManager sets and gets user tokens for implemented authentication mechanisms,
// and provides the data that is sent by the api and ui server after authenticating
type UserManager interface {
	GetUserByToken(token string) (User, error)
	CreateUserToken(username, password string) (string, error)
	// GetLoginHandler returns the function that starts the login process for auth mechanisms
	// that redirect to a thirdparty site for authentication
	GetLoginHandler(url string) func(http.ResponseWriter, *http.Request)
	// GetLoginRedirectHandler returns the function that does login for the
	// user once it has been redirected from a thirdparty site.
	GetLoginCallbackHandler() func(http.ResponseWriter, *http.Request)
	// IsRedirect returns true if the user must be redirected to a thirdparty site to authenticate
	IsRedirect() bool
}

type simpleUser struct {
	UserId       string
	Name         string
	EmailAddress string
}

func (u *simpleUser) DisplayName() string {
	return u.Name
}

func (u *simpleUser) Email() string {
	return u.EmailAddress
}

func (u *simpleUser) Username() string {
	return u.UserId
}

func (u *simpleUser) IsNil() bool {
	return u == nil
}
