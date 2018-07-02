package auth

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/crowd"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// CrowdUserManager handles authentication with Atlassian Crowd.
type CrowdUserManager struct {
	*crowd.Client
}

// NewCrowdUserManager creates a manager for the user and password combination that
// connects to the crowd service at the given URL.
func NewCrowdUserManager(conf *evergreen.CrowdConfig) (gimlet.UserManager, error) {
	crowdClient, err := crowd.NewClient(conf.Username, conf.Password, conf.Urlroot)
	if err != nil {
		return nil, err
	}
	return &CrowdUserManager{crowdClient}, nil
}

// GetUserByToken returns the user for the supplied token, or an
// error if the user is not found.
func (c *CrowdUserManager) GetUserByToken(_ context.Context, token string) (gimlet.User, error) {
	user, err := c.GetUserFromToken(token)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting user by token from crowd")
	}
	return &simpleUser{
		UserId:       user.Name,         //UserId
		Name:         user.DispName,     //Name
		EmailAddress: user.EmailAddress, //Email
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

func (*CrowdUserManager) GetLoginHandler(string) http.HandlerFunc    { return nil }
func (*CrowdUserManager) GetLoginCallbackHandler() http.HandlerFunc  { return nil }
func (*CrowdUserManager) IsRedirect() bool                           { return false }
func (*CrowdUserManager) GetUserByID(id string) (gimlet.User, error) { return getUserByID(id) }
func (*CrowdUserManager) GetOrCreateUser(u gimlet.User) (gimlet.User, error) {
	return getOrCreateUser(u)
}
