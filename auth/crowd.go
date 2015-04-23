package auth

import (
	"github.com/evergreen-ci/crowd"
)

type CrowdUserManager struct {
	*crowd.Client
}

func NewCrowdUserManager(user, pw, url string) (*CrowdUserManager, error) {
	crowdClient, err := crowd.NewClient(user, pw, url)
	if err != nil {
		return nil, err
	}
	return &CrowdUserManager{crowdClient}, nil
}

func (c *CrowdUserManager) GetUserByToken(token string) (MCIUser, error) {
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

func (c *CrowdUserManager) CreateUserToken(username, password string) (string, error) {
	session, err := c.CreateSession(username, password)
	if err != nil {
		return "", err
	}
	return session.Token, nil
}
