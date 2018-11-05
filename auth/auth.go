package auth

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

//LoadUserManager is used to check the configuration for authentication and create a UserManager
// depending on what type of authentication is used.
func LoadUserManager(authConfig evergreen.AuthConfig) (gimlet.UserManager, error) {
	var manager gimlet.UserManager
	var err error
	if authConfig.LDAP != nil {
		manager, err = NewLDAPUserManager(authConfig.LDAP)
		if err != nil {
			return nil, errors.Wrap(err, "problem setting up ldap authentication")
		}
		return manager, nil
	}
	if authConfig.Naive != nil {
		manager, err = NewNaiveUserManager(authConfig.Naive)
		if err != nil {
			return nil, errors.Wrap(err, "problem setting up naive authentication")
		}
		return manager, nil
	}
	if authConfig.Github != nil {
		manager, err = NewGithubUserManager(authConfig.Github)
		if err != nil {
			return nil, errors.Wrap(err, "problem setting up github authentication")
		}
		return manager, nil
	}
	return nil, errors.New("Must have at least one form of authentication, currently there are none")
}

// sets the Token in the session cookie for authentication
func setLoginToken(token string, w http.ResponseWriter) {
	authTokenCookie := &http.Cookie{
		Name:     evergreen.AuthTokenCookie,
		Value:    token,
		HttpOnly: true,
		Path:     "/",
	}
	http.SetCookie(w, authTokenCookie)
}

// IsSuperUser verifies that a given user has super user permissions.
// A user has these permission if they are in the super users list or if the list is empty,
// in which case all users are super users.
func IsSuperUser(superUsers []string, u gimlet.User) bool {
	if u == nil {
		return false
	}

	if util.StringSliceContains(superUsers, u.Username()) ||
		len(superUsers) == 0 {
		return true
	}
	return false

}

func getOrCreateUser(u gimlet.User) (gimlet.User, error) {
	return model.GetOrCreateUser(u.Username(), u.DisplayName(), u.Email())
}

func getUserByID(id string) (gimlet.User, error) { return model.FindUserByID(id) }
