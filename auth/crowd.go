package auth

import (
	"github.com/evergreen-ci/crowd"
	"github.com/evergreen-ci/evergreen"
	"net/http"
)

// CrowdUserManager handles authentication with Atlassian Crowd.
type CrowdUserManager struct {
	*crowd.Client
}

// NewCrowdUserManager creates a manager for the user and password combination that
// connects to the crowd service at the given URL.
func NewCrowdUserManager(conf *evergreen.CrowdConfig) (*CrowdUserManager, error) {
	crowdClient, err := crowd.NewClient(conf.Username, conf.Password, conf.Urlroot)
	if err != nil {
		return nil, err
	}
	return &CrowdUserManager{crowdClient}, nil
}

// GetUserByToken returns the user for the supplied token, or an
// error if the user is not found.
func (c *CrowdUserManager) GetUserByToken(token string) (User, error) {
	user, err := c.GetUserFromToken(token)
	if err != nil {
		return nil, err
	}
	return &simpleUser{
		user.Name,         //UserId
		user.DispName,     //Name
		user.EmailAddress, //Email
	}, nil
}

// CreateUserToken creates a user session in crowd. This session token is returned.
func (c *CrowdUserManager) CreateUserToken(username, password string) (string, error) {
	session, err := c.CreateSession(username, password)
	if err != nil {
		return "", err
	}
	return session.Token, nil
}

func (*CrowdUserManager) GetLoginHandler(string) func(http.ResponseWriter, *http.Request) {
	return nil
}

func (*CrowdUserManager) GetLoginCallbackHandler() func(http.ResponseWriter, *http.Request) {
	return nil
}

func (*CrowdUserManager) IsRedirect() bool {
	return false
}
