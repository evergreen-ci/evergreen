package ldap

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	ldap "gopkg.in/ldap.v2"
)

type LDAPSuite struct {
	um         gimlet.UserManager
	badGroupUm gimlet.UserManager
	realConnUm gimlet.UserManager
	suite.Suite
}

func TestLDAPSuite(t *testing.T) {
	suite.Run(t, new(LDAPSuite))
}

func (s *LDAPSuite) SetupTest() {
	var err error
	mockPutUser = nil
	s.um, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "10gen",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		connect:       mockConnect,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NotNil(s.um)
	s.Require().NoError(err)

	s.badGroupUm, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "badgroup",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		connect:       mockConnectErr,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NotNil(s.badGroupUm)
	s.Require().NoError(err)

	s.realConnUm, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "badgroup",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NotNil(s.badGroupUm)
	s.Require().NoError(err)
}

func mockConnect(url, port string) (ldap.Client, error) {
	return &mockConnSuccess{}, nil
}

func mockConnectErr(url, port string) (ldap.Client, error) {
	return &mockConnErr{}, nil
}

type mockConnSuccess struct {
	mockConn
}

type mockConnErr struct {
	mockConn
}

type mockConn struct{}

func (m *mockConn) Start()                            { return }
func (m *mockConn) StartTLS(config *tls.Config) error { return nil }
func (m *mockConn) Close()                            { return }
func (m *mockConn) SetTimeout(time.Duration)          { return }
func (m *mockConn) Bind(username, password string) error {
	if username == "uid=foo,path" && password == "hunter2" {
		return nil
	}
	return errors.Errorf("failed to Bind (%s, %s)", username, password)
}
func (m *mockConn) SimpleBind(simpleBindRequest *ldap.SimpleBindRequest) (*ldap.SimpleBindResult, error) {
	return nil, nil
}
func (m *mockConn) Add(addRequest *ldap.AddRequest) error             { return nil }
func (m *mockConn) Del(delRequest *ldap.DelRequest) error             { return nil }
func (m *mockConn) Modify(modifyRequest *ldap.ModifyRequest) error    { return nil }
func (m *mockConn) Compare(dn, attribute, value string) (bool, error) { return false, nil }
func (m *mockConn) PasswordModify(passwordModifyRequest *ldap.PasswordModifyRequest) (*ldap.PasswordModifyResult, error) {
	return nil, nil
}

func (m *mockConnSuccess) Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
	return &ldap.SearchResult{
		Entries: []*ldap.Entry{
			&ldap.Entry{
				Attributes: []*ldap.EntryAttribute{
					&ldap.EntryAttribute{
						Values: []string{"10gen"},
					},
					&ldap.EntryAttribute{
						Name:   "uid",
						Values: []string{"foo"},
					},
					&ldap.EntryAttribute{
						Name:   "mail",
						Values: []string{"foo@example.com"},
					},
					&ldap.EntryAttribute{
						Name:   "cn",
						Values: []string{"Foo Bar"},
					},
				},
			},
		},
	}, nil
}

func (m *mockConnErr) Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
	return nil, errors.New("mockConnErr")
}

func (m *mockConn) SearchWithPaging(searchRequest *ldap.SearchRequest, pagingSize uint32) (*ldap.SearchResult, error) {
	return nil, nil
}

type mockUser struct{ name string }

func (u *mockUser) DisplayName() string { return "" }
func (u *mockUser) Email() string       { return "" }
func (u *mockUser) Username() string    { return u.name }
func (u *mockUser) GetAPIKey() string   { return "" }
func (u *mockUser) Roles() []string     { return []string{} }

// TODO
var mockPutUser gimlet.User

func mockPutErr(u gimlet.User) (string, error) {
	return "", errors.New("putErr")
}

func mockPutSuccess(u gimlet.User) (string, error) {
	mockPutUser = u
	if u.Username() == "badUser" {
		return "", errors.New("got bad user")
	}
	return "123456", nil
}

func mockGetErr(token string) (gimlet.User, bool, error) {
	return nil, false, errors.New("error getting user")
}

func mockGetValid(token string) (gimlet.User, bool, error) {
	return &mockUser{name: token}, true, nil
}

func mockGetExpired(token string) (gimlet.User, bool, error) {
	return &mockUser{name: token}, false, nil
}

func mockGetMissing(token string) (gimlet.User, bool, error) {
	return nil, false, nil
}

func mockGetUserByID(id string) (gimlet.User, error) {
	u := gimlet.NewBasicUser("foo", "", "", "", []string{})
	return u, nil
}

func mockGetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	u := gimlet.NewBasicUser("foo", "", "", "", []string{})
	return u, nil
}

func (s *LDAPSuite) TestLDAPConstructorRequiresNonEmptyArgs() {
	l, err := NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.NotNil(l)
	s.NoError(err)

	l, err = NewUserService(CreationOpts{
		URL:           "",
		Port:          "port",
		Path:          "path",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "",
		Path:          "path",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "group",
		PutCache:      nil,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      nil,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       nil,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Nil(l)
	s.Error(err)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		Path:          "path",
		Group:         "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		GetUser:       mockGetUserByID,
		GetCreateUser: nil,
	})
	s.Nil(l)
	s.Error(err)
}

func (s *LDAPSuite) TestGetUserByToken() {
	ctx := context.Background()

	impl, ok := s.um.(*userService)
	s.True(ok)
	impl.GetCache = mockGetErr
	u, err := s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	impl.GetCache = mockGetValid
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	impl.GetCache = mockGetExpired
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	badGroupImpl, ok := s.badGroupUm.(*userService)
	s.True(ok)
	badGroupImpl.GetCache = mockGetExpired
	u, err = s.badGroupUm.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	u, err = s.um.GetUserByToken(ctx, "badUser")
	s.Error(err)
	s.Nil(u)

	impl.GetCache = mockGetMissing
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	realConnImpl, ok := s.realConnUm.(*userService)
	s.True(ok)
	realConnImpl.GetCache = mockGetExpired
	u, err = s.realConnUm.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)
}

func (s *LDAPSuite) TestCreateUserToken() {
	token, err := s.um.CreateUserToken("foo", "badpassword")
	s.Error(err)

	token, err = s.um.CreateUserToken("nosuchuser", "")
	s.Error(err)

	token, err = s.um.CreateUserToken("foo", "hunter2")
	s.NoError(err)
	s.Equal("123456", token)
	s.Equal("foo", mockPutUser.Username())
	s.Equal("Foo Bar", mockPutUser.DisplayName())
	s.Equal("foo@example.com", mockPutUser.Email())

	token, err = s.badGroupUm.CreateUserToken("foo", "hunter2")
	s.Error(err)
	s.Empty(token)

	impl, ok := s.um.(*userService)
	s.True(ok)
	impl.PutCache = mockPutErr
	token, err = s.um.CreateUserToken("foo", "hunter2")
	s.Error(err)
	s.Empty(token)

	token, err = s.um.CreateUserToken("foo", "hunter2")
	s.Error(err)
	s.Empty(token)

	token, err = s.realConnUm.CreateUserToken("foo", "hunter2")
	s.Empty(token)
	s.Error(err)
}

func (s *LDAPSuite) TestGetLoginHandler() {
	s.Nil(s.um.GetLoginHandler(""))
}

func (s *LDAPSuite) TestGetLoginCallbackHandler() {
	s.Nil(s.um.GetLoginCallbackHandler())
}

func (s *LDAPSuite) TestIsRedirect() {
	s.False(s.um.IsRedirect())
}

func (s *LDAPSuite) TestGetUser() {
	user, err := s.um.GetUserByID("foo")
	s.NoError(err)
	s.Equal("foo", user.Username())
}

func (s *LDAPSuite) TestGetOrCreateUser() {
	basicUser := gimlet.MakeBasicUser()
	user, err := s.um.GetOrCreateUser(basicUser)
	s.NoError(err)
	s.Equal("foo", user.Username())
}
