package gimlet

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterfacesAreImplemented(t *testing.T) {
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
	Password     string
	Token        string
	ReportNil    bool
	APIKey       string
	AccessToken  string
	RefreshToken string
	RoleNames    []string
	Groups       []string
}

func (u *MockUser) DisplayName() string     { return u.Name }
func (u *MockUser) Email() string           { return u.EmailAddress }
func (u *MockUser) Username() string        { return u.ID }
func (u *MockUser) IsNil() bool             { return u.ReportNil }
func (u *MockUser) GetAPIKey() string       { return u.APIKey }
func (u *MockUser) GetAccessToken() string  { return u.AccessToken }
func (u *MockUser) GetRefreshToken() string { return u.RefreshToken }
func (u *MockUser) Roles() []string         { return u.RoleNames }
func (u *MockUser) HasPermission(PermissionOpts) bool {
	return true
}

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
	Users                []*MockUser
	FailGetOrCreateUser  bool
	FailGetUserByToken   bool
	FailCreateUserToken  bool
	FailGetUserByID      bool
	FailClearUser        bool
	FailGetGroupsForUser bool
	FailReauthorizeUser  bool
	Redirect             bool
	LoginHandler         http.HandlerFunc
	LoginCallbackHandler http.HandlerFunc
}

func (m *MockUserManager) GetUserByToken(_ context.Context, token string) (User, error) {
	if m.FailGetUserByToken {
		return nil, errors.New("mock fail")
	}
	for _, u := range m.Users {
		if token == u.Token {
			return u, nil
		}
	}
	return nil, errors.New("user not found")
}

func mockUserToken(username, password string) string {
	return strings.Join([]string{username, password}, ".")
}

func (m *MockUserManager) CreateUserToken(username, password string) (string, error) {
	if m.FailCreateUserToken {
		return "", errors.New("mock fail")
	}
	return mockUserToken(username, password), nil
}

func (m *MockUserManager) GetLoginHandler(url string) http.HandlerFunc { return m.LoginHandler }
func (m *MockUserManager) GetLoginCallbackHandler() http.HandlerFunc   { return m.LoginCallbackHandler }
func (m *MockUserManager) IsRedirect() bool                            { return m.Redirect }
func (m *MockUserManager) ReauthorizeUser(user User) error {
	if m.FailReauthorizeUser {
		return errors.New("mock fail")
	}
	for _, u := range m.Users {
		if user.Username() == u.Username() {
			return nil
		}
	}
	return errors.New("user not found")
}

func (m *MockUserManager) GetUserByID(id string) (User, error) {
	if m.FailGetUserByID {
		return nil, errors.New("mock fail")
	}
	for _, u := range m.Users {
		if id == u.Username() {
			return u, nil
		}
	}
	return nil, errors.New("user does not exist")
}

func (m *MockUserManager) GetOrCreateUser(u User) (User, error) {
	if m.FailGetOrCreateUser {
		return nil, errors.New("mock fail")
	}

	return u, nil
}

func (m *MockUserManager) ClearUser(u User, all bool) error {
	if m.FailClearUser {
		return errors.New("mock fail")
	}
	return nil
}

func (m *MockUserManager) GetGroupsForUser(username string) ([]string, error) {
	if m.FailGetGroupsForUser {
		return nil, errors.New("mock fail")
	}
	for _, u := range m.Users {
		if username == u.Username() {
			return u.Groups, nil
		}
	}
	return nil, errors.New("not found")
}
