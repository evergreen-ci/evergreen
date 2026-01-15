package testutil

import (
	"context"
	"errors"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
)

// MockUserManager is used for testing the servers.
// It accepts all tokens and return the same user for all tokens.
type MockUserManager struct{}

var MockUser = user.DBUser{Id: "testuser", APIKey: "testapikey"}
var MockServiceUser = user.DBUser{Id: "serviceuser", APIKey: "serviceapikey", OnlyAPI: true}

func (MockUserManager) GetUserByToken(_ context.Context, _ string) (gimlet.User, error) {
	return &MockUser, nil
}
func (MockUserManager) CreateUserToken(_ context.Context, _, _ string) (string, error) {
	return MockUser.Username(), nil
}
func (MockUserManager) GetLoginHandler(_ string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {}
}
func (MockUserManager) IsRedirect() bool { return false }
func (MockUserManager) GetLoginCallbackHandler() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {}
}
func (MockUserManager) ReauthorizeUser(context.Context, gimlet.User) error {
	return errors.New("not implemented")
}
func (MockUserManager) GetOrCreateUser(context.Context, gimlet.User) (gimlet.User, error) {
	return &MockUser, nil
}
func (MockUserManager) GetUserByID(_ context.Context, id string) (gimlet.User, error) {
	if id == MockServiceUser.Id {
		return &MockServiceUser, nil
	}
	return &MockUser, nil
}
func (MockUserManager) ClearUser(context.Context, gimlet.User, bool) error {
	return errors.New("MockUserManager does not support Clear User")
}
func (MockUserManager) GetGroupsForUser(string) ([]string, error) { return []string{}, nil }
