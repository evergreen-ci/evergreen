package auth

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
)

// NewOnlyAPIUserManager creates a user manager for special users that can only
// make API requests. Users are pre-populated from the given config.
func NewOnlyAPIUserManager(config *evergreen.OnlyAPIAuthConfig) (gimlet.UserManager, error) {
	env := evergreen.GetEnvironment()
	var rm gimlet.RoleManager
	if env != nil {
		rm = env.RoleManager()
	}

	users := make([]gimlet.BasicUser, 0, len(config.Users))
	catcher := grip.NewBasicCatcher()
	for _, u := range config.Users {
		opts, err := gimlet.NewBasicUserOptions(u.Username)
		if err != nil {
			catcher.Wrap(err, "invalid API-only user in config")
			continue
		}
		users = append(users, *gimlet.NewBasicUser(opts.Key(u.Key).RoleManager(rm)))
	}

	return gimlet.NewBasicUserManager(users, rm)
}
