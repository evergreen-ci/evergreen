package gimlet

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterfacesAteImplemented(t *testing.T) {
	assert := assert.New(t)

	assert.Implements((*Authenticator)(nil), &MockAuthenticator{})
	assert.Implements((*UserManager)(nil), &MockUserManager{})
	assert.Implements((*User)(nil), &MockUser{})

}

////////////////////////////////////////////////////////////////////////
//
// Mock Implementations

type MockUser struct {
	ID           string
	Name         string
	EmailAddress string
	ReportNil    bool
	APIKey       string
	RoleNames    []string
}

func (u *MockUser) DisplayName() string { return u.Name }
func (u *MockUser) Email() string       { return u.EmailAddress }
func (u *MockUser) Username() string    { return u.ID }
func (u *MockUser) IsNil() bool         { return u.ReportNil }
func (u *MockUser) GetAPIKey() string   { return u.APIKey }
func (u *MockUser) Roles() []string     { return u.RoleNames }

type MockAuthenticator struct {
	ResourceUserMapping     map[string]string
	GroupUserMapping        map[string]string
	CheckAuthenticatedState map[string]bool
	UserToken               string
}

func (a *MockAuthenticator) CheckResourceAccess(u User, resource string) bool {
	r, ok := a.ResourceUserMapping[u.Username()]
	if !ok {
		return false
	}

	return r == resource
}

func (a *MockAuthenticator) CheckGroupAccess(u User, group string) bool {
	g, ok := a.GroupUserMapping[u.Username()]
	if !ok {
		return false
	}

	return g == group
}
func (a *MockAuthenticator) CheckAuthenticated(u User) bool {
	return a.CheckAuthenticatedState[u.Username()]
}

func (a *MockAuthenticator) GetUserFromRequest(um UserManager, r *http.Request) (User, error) {
	ctx := r.Context()

	u, err := um.GetUserByToken(ctx, a.UserToken)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errors.New("user not defined")
	}
	return u, nil
}

type MockUserManager struct {
	TokenToUsers    map[string]User
	CreateUserFails bool
}

func (m *MockUserManager) GetUserByToken(_ context.Context, token string) (User, error) {
	if m.TokenToUsers == nil {
		return nil, errors.New("no users configured")
	}

	u, ok := m.TokenToUsers[token]
	if !ok {
		return nil, errors.New("user does not exist")
	}

	return u, nil
}

func (m *MockUserManager) CreateUserToken(username, password string) (string, error) {
	return strings.Join([]string{username, password}, "."), nil
}

func (m *MockUserManager) GetLoginHandler(url string) http.HandlerFunc { return nil }
func (m *MockUserManager) GetLoginCallbackHandler() http.HandlerFunc   { return nil }
func (m *MockUserManager) IsRedirect() bool                            { return false }

func (m *MockUserManager) GetUserByID(id string) (User, error) {
	u, ok := m.TokenToUsers[id]
	if !ok {
		return nil, errors.New("not exist")
	}

	return u, nil
}
func (m *MockUserManager) GetOrCreateUser(u User) (User, error) {
	if m.CreateUserFails {
		return nil, errors.New("event")
	}

	return u, nil
}

func (m *MockUserManager) ClearUser(u User, all bool) error {
	return errors.New("MockUserManager does not support Clear User")
}
