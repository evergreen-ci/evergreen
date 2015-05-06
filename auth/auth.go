package auth

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
)

//LoadUserManager is used to check the configuration for authentication and create a UserManager depending on what type of authentication (Crowd or Naive) is used.
func LoadUserManager(authConfig evergreen.AuthConfig) (UserManager, error) {
	var manager UserManager
	var err error
	if authConfig.Crowd != nil {
		manager, err = NewCrowdUserManager(authConfig.Crowd.Username, authConfig.Crowd.Password, authConfig.Crowd.Urlroot)
		if err != nil {
			return nil, err
		}
	}
	if authConfig.Naive != nil {
		if manager != nil {
			return nil, fmt.Errorf("Cannot have multiple forms of authentication in configuration")
		}
		manager, err = NewNaiveUserManager(authConfig.Naive)
		if err != nil {
			return nil, err
		}
	}
	if manager != nil {
		return manager, nil
	}

	return nil, fmt.Errorf("Must have at least one form of authentication, currently there are none")

}
