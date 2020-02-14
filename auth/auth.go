package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var userManager struct {
	manager        gimlet.UserManager
	canClearTokens bool
	mutex          sync.RWMutex
}

// InitUserManager initializes the global user manager if it does not exist
// already based on the given settings. If it already exists, it is returned.
func InitUserManager(settings *evergreen.Settings) (um gimlet.UserManager, supportsClearTokens bool, err error) {
	userManager.mutex.RLock()
	if userManager.manager != nil {
		userManager.mutex.RUnlock()
		return userManager.manager, userManager.canClearTokens, nil
	}

	um, canClearTokens, err := LoadUserManager(settings)

	userManager.mutex.Lock()
	defer userManager.mutex.Unlock()
	userManager.manager = um
	userManager.canClearTokens = canClearTokens

	return um, canClearTokens, err
}

// UserManager returns the global user manager.
func UserManager() (gimlet.UserManager, error) {
	if userManager.manager == nil {
		return nil, errors.New("user manager has not been initialized")
	}
	return userManager.manager, nil
}

// LoadUserManager is used to check the configuration for authentication and
// create a UserManager depending on what type of authentication is used. The
// global user manager is not set.
func LoadUserManager(settings *evergreen.Settings) (um gimlet.UserManager, supportsClearTokens bool, err error) {
	authConfig := settings.AuthConfig
	makeLDAPManager := func() (gimlet.UserManager, bool, error) {
		manager, err := NewLDAPUserManager(authConfig.LDAP)
		if err != nil {
			return nil, false, errors.Wrap(err, "problem setting up ldap authentication")
		}
		return manager, true, nil
	}
	makeOktaManager := func() (gimlet.UserManager, bool, error) {
		manager, err := NewOktaUserManager(authConfig.Okta, settings.Ui.Url, settings.Ui.LoginDomain)
		if err != nil {
			return nil, false, errors.Wrap(err, "problem setting up okta authentication")
		}
		return manager, true, nil
	}
	makeNaiveManager := func() (gimlet.UserManager, bool, error) {
		manager, err := NewNaiveUserManager(authConfig.Naive)
		if err != nil {
			return nil, false, errors.Wrap(err, "problem setting up naive authentication")
		}
		return manager, false, nil
	}
	makeGithubManager := func() (gimlet.UserManager, bool, error) {
		manager, err := NewGithubUserManager(authConfig.Github, settings.Ui.LoginDomain)
		if err != nil {
			return nil, false, errors.Wrap(err, "problem setting up github authentication")
		}
		return manager, false, nil
	}

	switch authConfig.PreferredType {
	case evergreen.AuthLDAPKey:
		if authConfig.LDAP != nil {
			return makeLDAPManager()
		}
	case evergreen.AuthOktaKey:
		if authConfig.Okta != nil {
			return makeOktaManager()
		}
	case evergreen.AuthGithubKey:
		if authConfig.Github != nil {
			return makeGithubManager()
		}
	case evergreen.AuthNaiveKey:
		if authConfig.Naive != nil {
			return makeNaiveManager()
		}
	}

	if authConfig.LDAP != nil {
		return makeLDAPManager()
	}
	if authConfig.Okta != nil {
		return makeOktaManager()
	}
	if authConfig.Naive != nil {
		return makeNaiveManager()
	}
	if authConfig.Github != nil {
		return makeGithubManager()
	}
	return nil, false, errors.New("Must have at least one form of authentication, currently there are none")
}

// SetLoginToken sets the token in the session cookie for authentication.
func SetLoginToken(token, domain string, w http.ResponseWriter) {
	settings, err := evergreen.GetConfig()
	if err != nil {
		grip.Error(err)
	}
	if settings.Ui.ExpireLoginCookieDomain != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     evergreen.AuthTokenCookie,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Now().Add(-1 * time.Hour),
			Domain:   settings.Ui.ExpireLoginCookieDomain,
		})
	}
	authTokenCookie := &http.Cookie{
		Name:     evergreen.AuthTokenCookie,
		Value:    token,
		HttpOnly: true,
		Path:     "/",
		Domain:   domain,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
	}
	http.SetCookie(w, authTokenCookie)
}

func getOrCreateUser(u gimlet.User) (gimlet.User, error) {
	return model.GetOrCreateUser(u.Username(), u.DisplayName(), u.Email(), u.GetAccessToken(), u.GetRefreshToken())
}

func getUserByID(id string) (gimlet.User, error) { return model.FindUserByID(id) }

// getUserWithExpiration returns a user by id and a boolean. True indicates the user is valid. False
// indicates that the user has expired. An error is returned if the user does not exist or if there
// is an error retrieving the user.
func getUserByIdWithExpiration(id string, expireAfter time.Duration) (gimlet.User, bool, error) {
	u, err := model.FindUserByID(id)
	if err != nil {
		return nil, false, errors.Wrap(err, "problem getting user from cache")
	}
	if time.Since(u.LoginCache.TTL) > expireAfter {
		return u, false, nil
	}
	return u, true, nil
}
