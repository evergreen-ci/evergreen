package auth

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/ldap"
	"github.com/pkg/errors"
)

const expireAfter = 24 * time.Hour

// NewLDAPUserManager creates a user manager for an LDAP server.
func NewLDAPUserManager(conf *evergreen.LDAPConfig) (gimlet.UserManager, error) {
	opts := ldap.CreationOpts{
		URL:           conf.URL,
		Port:          conf.Port,
		Path:          conf.Path,
		Group:         conf.Group,
		PutCache:      func(u gimlet.User) (string, error) { return user.PutLoginCache(u) },
		GetCache:      func(token string) (gimlet.User, bool, error) { return user.GetLoginCache(token, expireAfter) },
		GetUser:       func(id string) (gimlet.User, error) { return getUserByID(id) },
		GetCreateUser: func(u gimlet.User) (gimlet.User, error) { return getOrCreateUser(u) },
	}
	um, err := ldap.NewUserService(opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not construct LDAP user manager")
	}
	return um, nil
}
