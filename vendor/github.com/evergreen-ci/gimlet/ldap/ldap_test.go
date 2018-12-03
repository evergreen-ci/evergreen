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
	um             gimlet.UserManager
	badGroupUm     gimlet.UserManager
	serviceGroupUm gimlet.UserManager
	realConnUm     gimlet.UserManager
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
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "10gen",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		connect:       mockConnect,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.um)

	s.serviceGroupUm, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "badgroup",
		ServiceGroup:  "10gen",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		connect:       mockConnect,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.um)

	s.badGroupUm, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "badgroup",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		connect:       mockConnectErr,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.badGroupUm)

	s.realConnUm, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "badgroup",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.badGroupUm)
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
	if username == "uid=foo,bots" && password == "hunter3" {
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

func mockClearCache(u gimlet.User, all bool) error {
	return nil
}

func mockGetUserByID(id string) (gimlet.User, error) {
	u := gimlet.NewBasicUser(id, "", "", "", []string{})
	return u, nil
}

func mockGetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	u := gimlet.NewBasicUser(user.Username(), user.DisplayName(), user.Email(), user.GetAPIKey(), []string{})
	return u, nil
}

func (s *LDAPSuite) TestLDAPConstructorRequiresNonEmptyArgs() {
	l, err := NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.NoError(err)
	s.NotNil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      nil,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      nil,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    nil,
		GetUser:       mockGetUserByID,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       nil,
		GetCreateUser: mockGetOrCreateUser,
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:           "url",
		Port:          "port",
		UserPath:      "path",
		ServicePath:   "bots",
		UserGroup:     "group",
		PutCache:      mockPutSuccess,
		GetCache:      mockGetValid,
		ClearCache:    mockClearCache,
		GetUser:       mockGetUserByID,
		GetCreateUser: nil,
	})
	s.Error(err)
	s.Nil(l)
}

func (s *LDAPSuite) TestGetUserByToken() {
	ctx := context.Background()

	impl, ok := s.um.(*userService).cache.(*externalUserCache)
	s.True(ok)
	impl.get = mockGetErr
	u, err := s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	impl.get = mockGetValid
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	impl.get = mockGetExpired
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	serviceGroupImpl, ok := s.serviceGroupUm.(*userService).cache.(*externalUserCache)
	s.True(ok)
	serviceGroupImpl.get = mockGetExpired
	u, err = s.badGroupUm.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	badGroupImpl, ok := s.badGroupUm.(*userService).cache.(*externalUserCache)
	s.True(ok)
	badGroupImpl.get = mockGetExpired
	u, err = s.badGroupUm.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	u, err = s.um.GetUserByToken(ctx, "badUser")
	s.Error(err)
	s.Nil(u)

	impl.get = mockGetMissing
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	realConnImpl, ok := s.realConnUm.(*userService).cache.(*externalUserCache)
	s.True(ok)
	realConnImpl.get = mockGetExpired
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

	impl, ok := s.um.(*userService).cache.(*externalUserCache)
	s.True(ok)
	impl.put = mockPutErr
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
	basicUser := gimlet.NewBasicUser("foo", "", "", "", []string{})
	user, err := s.um.GetOrCreateUser(basicUser)
	s.NoError(err)
	s.Equal("foo", user.Username())
}

func (s *LDAPSuite) TestLoginUsesBothPaths() {
	userManager, ok := s.um.(*userService)
	s.True(ok)
	s.NotNil(userManager)
	s.Error(userManager.login("foo", "hunter1"))
	s.NoError(userManager.login("foo", "hunter2"))
	s.NoError(userManager.login("foo", "hunter3"))
}

func (s *LDAPSuite) TestClearCache() {
	basicUser := gimlet.NewBasicUser("foo", "", "", "", []string{})
	user, err := s.um.GetOrCreateUser(basicUser)
	s.Require().NoError(err)

	err = s.um.ClearUser(user, false)
	s.NoError(err)
}
