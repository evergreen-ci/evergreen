package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/user"
)

// MockUserManager is used for testing the servers.
// It accepts all tokens and return the same user for all tokens.
type MockUserManager struct{}

var MockUser = user.DBUser{Id: "testuser"}

func (MockUserManager) GetUserByToken(_ string) (auth.User, error)  { return &MockUser, nil }
func (MockUserManager) CreateUserToken(_, _ string) (string, error) { return MockUser.Username(), nil }
func (MockUserManager) GetLoginHandler(_ string) func(http.ResponseWriter, *http.Request) {
	return nil
}
func (MockUserManager) IsRedirect() bool                                                  { return false }
func (MockUserManager) GetLoginCallbackHandler() func(http.ResponseWriter, *http.Request) { return nil }
