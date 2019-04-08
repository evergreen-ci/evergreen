package auth

import (
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/ldap"
	"github.com/pkg/errors"
)

// NewLDAPUserManager creates a user manager for an LDAP server.
func NewLDAPUserManager(conf *evergreen.LDAPConfig) (gimlet.UserManager, error) {
	minutes, err := strconv.ParseInt(conf.ExpireAfterMinutes, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "problem parsing string as int '%s'", conf.ExpireAfterMinutes)
	}
	expireAfter := time.Duration(minutes) * time.Minute
	opts := ldap.CreationOpts{
		URL:           conf.URL,
		Port:          conf.Port,
		UserPath:      conf.UserPath,
		ServicePath:   conf.ServicePath,
		UserGroup:     conf.Group,
		ServiceGroup:  conf.ServiceGroup,
		PutCache:      user.PutLoginCache,
		GetCache:      func(token string) (gimlet.User, bool, error) { return user.GetLoginCache(token, expireAfter) },
		ClearCache:    user.ClearLoginCache,
		GetUser:       func(id string) (gimlet.User, bool, error) { return getUserByIdWithExpiration(id, expireAfter) },
		GetCreateUser: getOrCreateUser,
	}
	um, err := ldap.NewUserService(opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not construct LDAP user manager")
	}
	return um, nil
}
