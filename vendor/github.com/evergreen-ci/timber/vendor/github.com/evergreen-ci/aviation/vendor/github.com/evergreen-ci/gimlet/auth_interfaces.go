package gimlet

import (
	"context"
	"net/http"
)

// User provides a common way of interacting with users from
// authentication systems.
//
// Note: this is the User interface from Evergreen with the addition
// of the Roles() method.
type User interface {
	DisplayName() string
	Email() string
	Username() string
	GetAPIKey() string
	Roles() []string
}

// Authenticator represents a service that answers specific
// authentication related questions, and is the public interface
// used for authentication workflows.
type Authenticator interface {
	CheckResourceAccess(User, string) bool
	CheckGroupAccess(User, string) bool
	CheckAuthenticated(User) bool
}

// UserManager sets and gets user tokens for implemented
// authentication mechanisms, and provides the data that is sent by
// the api and ui server after authenticating
type UserManager interface {
	// The first 5 methods are borrowed directly from evergreen without modification
	GetUserByToken(context.Context, string) (User, error)
	CreateUserToken(string, string) (string, error)
	// GetLoginHandler returns the function that starts the login process for auth mechanisms
	// that redirect to a thirdparty site for authentication
	GetLoginHandler(url string) http.HandlerFunc
	// GetLoginRedirectHandler returns the function that does login for the
	// user once it has been redirected from a thirdparty site.
	GetLoginCallbackHandler() http.HandlerFunc

	// IsRedirect returns true if the user must be redirected to a
	// thirdparty site to authenticate.
	// TODO: should add a "do redirect if needed".
	IsRedirect() bool

	// These methods are simple wrappers around the user
	// persistence layer. May consider moving them to the
	// authenticator.
	GetUserByID(string) (User, error)
	GetOrCreateUser(User) (User, error)

	// Log out user or all users
	ClearUser(user User, all bool) error
}
