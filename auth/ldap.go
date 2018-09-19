package auth

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/ldap"
	"github.com/pkg/errors"
)

// NewLDAPUserManager creates a user manager for an LDAP server.
func NewLDAPUserManager(conf *evergreen.LDAPConfig) (gimlet.UserManager, error) {
	expireAfter := time.Duration(conf.ExpireAfterMinutes) * time.Minute
	opts := ldap.CreationOpts{
		URL:           conf.URL,
		Port:          conf.Port,
		Path:          conf.Path,
		Group:         conf.Group,
		PutCache:      user.PutLoginCache,
		GetCache:      func(token string) (gimlet.User, bool, error) { return user.GetLoginCache(token, expireAfter) },
		GetUser:       getUserByID,
		GetCreateUser: getOrCreateUser,
	}
	um, err := ldap.NewUserService(opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not construct LDAP user manager")
	}
	return um, nil
}
