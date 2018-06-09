package testutil

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
)

// MockUserManager is used for testing the servers.
// It accepts all tokens and return the same user for all tokens.
type MockUserManager struct{}

var MockUser = user.DBUser{Id: "testuser"}

func (MockUserManager) GetUserByToken(_ context.Context, _ string) (auth.User, error) {
	return &MockUser, nil
}
func (MockUserManager) CreateUserToken(_, _ string) (string, error)      { return MockUser.Username(), nil }
func (MockUserManager) GetLoginHandler(_ string) http.HandlerFunc        { return nil }
func (MockUserManager) IsRedirect() bool                                 { return false }
func (MockUserManager) GetLoginCallbackHandler() http.HandlerFunc        { return nil }
func (MockUserManager) GetOrCreateUser(gimlet.User) (gimlet.User, error) { return nil, nil }
func (MockUserManager) GetUserByID(string) (gimlet.User, error)          { return nil, nil }
