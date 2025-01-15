package auth

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// LoadUserManager is used to check the configuration for authentication and
// create a UserManager depending on what type of authentication is used.
func LoadUserManager(settings *evergreen.Settings) (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	authConfig := settings.AuthConfig

	if authConfig.PreferredType != "" {
		if authConfig.PreferredType == evergreen.AuthMultiKey {
			manager, info, err := makeMultiManager(settings, authConfig)
			if err == nil {
				return manager, info, nil
			}
		} else {
			manager, info, err := makeSingleManager(authConfig.PreferredType, settings, authConfig)
			if err == nil {
				return manager, info, nil
			}
		}
	}

	if authConfig.Multi != nil && !authConfig.Multi.IsZero() {
		return makeMultiManager(settings, authConfig)
	}
	if authConfig.Okta != nil {
		return makeOktaManager(settings, authConfig.Okta)
	}
	if authConfig.Naive != nil {
		return makeNaiveManager(authConfig.Naive)
	}
	if authConfig.Github != nil && authConfig.Github.ClientId != "" && authConfig.Github.ClientSecret != "" && authConfig.Github.Organization != "" && len(authConfig.Github.Users) > 0 {
		return makeGithubManager(settings, authConfig.Github)
	}
	if authConfig.AllowServiceUsers {
		return makeOnlyAPIManager()
	}
	if authConfig.Kanopy != nil {
		return makeExternalManager()
	}
	return nil, evergreen.UserManagerInfo{}, errors.New("Must have at least one form of authentication, currently there are none")
}

func makeOktaManager(settings *evergreen.Settings, config *evergreen.OktaConfig) (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	manager, err := NewOktaUserManager(config, settings.Ui.Url, settings.Ui.LoginDomain)
	if err != nil {
		return nil, evergreen.UserManagerInfo{}, errors.Wrap(err, "problem setting up okta authentication")
	}
	return manager, evergreen.UserManagerInfo{
		CanClearTokens: true,
		CanReauthorize: true,
	}, nil
}

func makeNaiveManager(config *evergreen.NaiveAuthConfig) (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	manager, err := NewNaiveUserManager(config)
	if err != nil {
		return nil, evergreen.UserManagerInfo{}, errors.Wrap(err, "problem setting up naive authentication")
	}
	return manager, evergreen.UserManagerInfo{}, nil
}

func makeOnlyAPIManager() (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	manager, err := NewOnlyAPIUserManager()
	if err != nil {
		return nil, evergreen.UserManagerInfo{}, errors.Wrap(err, "problem setting up API-only authentication")
	}
	return manager, evergreen.UserManagerInfo{}, nil
}

func makeGithubManager(settings *evergreen.Settings, config *evergreen.GithubAuthConfig) (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	manager, err := NewGithubUserManager(config, settings.Ui.LoginDomain)
	if err != nil {
		return nil, evergreen.UserManagerInfo{}, errors.Wrap(err, "problem setting up github authentication")
	}
	return manager, evergreen.UserManagerInfo{}, nil
}

func makeExternalManager() (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	manager, err := NewExternalUserManager()
	if err != nil {
		return nil, evergreen.UserManagerInfo{}, errors.Wrap(err, "making external user manager")
	}
	return manager, evergreen.UserManagerInfo{CanClearTokens: false, CanReauthorize: false}, nil
}

func makeMultiManager(settings *evergreen.Settings, config evergreen.AuthConfig) (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	var multiInfo evergreen.UserManagerInfo
	var rw []gimlet.UserManager
	for _, kind := range config.Multi.ReadWrite {
		manager, info, err := makeSingleManager(kind, settings, config)
		if err != nil {
			grip.Critical(errors.Wrapf(err, "could not create '%s' manager", kind))
			continue
		}
		rw = append(rw, manager)
		if info.CanClearTokens {
			multiInfo.CanClearTokens = true
		}
		if info.CanReauthorize {
			multiInfo.CanReauthorize = true
		}
	}
	var ro []gimlet.UserManager
	if config.AllowServiceUsers {
		manager, _, err := makeOnlyAPIManager()
		if err == nil {
			ro = append(ro, manager)
		} else {
			grip.Critical(errors.Wrap(err, "could not create only-api manager"))
		}
	}
	for _, kind := range config.Multi.ReadOnly {
		// Ignore info because these managers are read-only and info currently
		// only contains write operation capabilities.
		manager, _, err := makeSingleManager(kind, settings, config)
		if err != nil {
			grip.Critical(errors.Wrapf(err, "could not create '%s' manager", kind))
			continue
		}
		ro = append(ro, manager)
	}
	multi := gimlet.NewMultiUserManager(rw, ro)
	return multi, multiInfo, nil
}

func makeSingleManager(kind string, settings *evergreen.Settings, config evergreen.AuthConfig) (gimlet.UserManager, evergreen.UserManagerInfo, error) {
	switch kind {
	case evergreen.AuthOktaKey:
		if config.Okta != nil {
			return makeOktaManager(settings, config.Okta)
		}
	case evergreen.AuthGithubKey:
		if config.Github != nil {
			return makeGithubManager(settings, config.Github)
		}
	case evergreen.AuthNaiveKey:
		if config.Naive != nil {
			return makeNaiveManager(config.Naive)
		}
	case evergreen.AuthAllowServiceUsersKey:
		return makeOnlyAPIManager()
	case evergreen.AuthKanopyKey:
		if config.Kanopy != nil {
			return makeExternalManager()
		}
	default:
		return nil, evergreen.UserManagerInfo{}, errors.Errorf("unrecognized user manager of type '%s'", kind)
	}
	return nil, evergreen.UserManagerInfo{}, errors.Errorf("user manager of type '%s' could not be created", kind)
}

// SetLoginToken sets the token in the session cookie for authentication.
func SetLoginToken(token, domain string, w http.ResponseWriter) {
	authTokenCookie := &http.Cookie{
		Name:     evergreen.AuthTokenCookie,
		Value:    token,
		HttpOnly: true,
		Path:     "/",
		Domain:   domain,
		Expires:  time.Now().Add(evergreen.LoginCookieTTL),
		Secure:   true,
	}
	http.SetCookie(w, authTokenCookie)
}

func getOrCreateUser(u gimlet.User) (gimlet.User, error) {
	return user.GetOrCreateUser(u.Username(), u.DisplayName(), u.Email(), u.GetAccessToken(), u.GetRefreshToken(), u.Roles())
}

func getUserByID(id string) (gimlet.User, error) {
	u, err := user.FindOneById(id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.Errorf("user with id '%s' not found", id)
	}
	return u, nil
}

// getUserWithExpiration returns a user by id and a boolean. True indicates the user is valid. False
// indicates that the user has expired. An error is returned if the user does not exist or if there
// is an error retrieving the user.
func getUserByIdWithExpiration(id string, expireAfter time.Duration) (gimlet.User, bool, error) {
	u, err := user.FindOneById(id)
	if err != nil {
		return nil, false, errors.Wrap(err, "problem getting user from cache")
	}
	if u == nil {
		return nil, false, errors.Errorf("user with id '%s' not found", id)
	}
	if time.Since(u.LoginCache.TTL) > expireAfter {
		return u, false, nil
	}
	return u, true, nil
}
