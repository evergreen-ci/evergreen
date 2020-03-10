package ldap

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	ldap "gopkg.in/ldap.v3"
)

type LDAPSuite struct {
	um             gimlet.UserManager
	badGroupUm     gimlet.UserManager
	serviceGroupUm gimlet.UserManager
	serviceUserUm  gimlet.UserManager
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
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "cn=10gen,ou=groups,dc=mongodb,dc=com",
		GroupOuName: "groups",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
		connect: mockConnect,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.um)

	s.serviceGroupUm, err = NewUserService(CreationOpts{
		URL:          "url",
		Port:         "port",
		UserPath:     "path",
		ServicePath:  "bots",
		UserGroup:    "badgroup",
		ServiceGroup: "cn=10gen,ou=groups,dc=mongodb,dc=com",
		GroupOuName:  "groups",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
		connect: mockConnect,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.um)

	s.badGroupUm, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "badgroup",
		GroupOuName: "groups",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
		connect: mockConnectErr,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.badGroupUm)

	s.serviceUserUm, err = NewUserService(CreationOpts{
		URL:                 "url",
		Port:                "port",
		UserPath:            "path",
		ServicePath:         "bots",
		UserGroup:           "badgroup",
		ServiceGroup:        "cn=10gen,ou=groups,dc=mongodb,dc=com",
		GroupOuName:         "groups",
		ServiceUserName:     "service_user",
		ServiceUserPassword: "service_password",
		ServiceUserPath:     "service_path",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
		connect: mockConnect,
	})
	s.Require().NoError(err)
	s.Require().NotNil(s.serviceUserUm)

	s.realConnUm, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "badgroup",
		GroupOuName: "groups",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
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

func (m *mockConn) Start()                               {}
func (m *mockConn) StartTLS(config *tls.Config) error    { return nil }
func (m *mockConn) Close()                               {}
func (m *mockConn) SetTimeout(time.Duration)             {}
func (m *mockConn) ModifyDN(*ldap.ModifyDNRequest) error { return nil }
func (m *mockConn) Bind(username, password string) error {
	if username == "uid=foo,path" && password == "hunter2" {
		return nil
	}
	if username == "uid=foo,bots" && password == "hunter3" {
		return nil
	}
	if username == "uid=service_user,service_path" && password == "service_password" {
		return nil
	}
	return errors.Errorf("failed to Bind (%s, %s)", username, password)
}
func (m *mockConn) UnauthenticatedBind(username string) error { return nil }
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

func (m *mockConn) ExternalBind() error { return nil }

func (m *mockConnSuccess) Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
	return &ldap.SearchResult{
		Entries: []*ldap.Entry{
			&ldap.Entry{
				Attributes: []*ldap.EntryAttribute{
					&ldap.EntryAttribute{
						Values: []string{"cn=10gen,ou=groups,dc=mongodb,dc=com"},
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

func (u *mockUser) DisplayName() string     { return "" }
func (u *mockUser) Email() string           { return "" }
func (u *mockUser) Username() string        { return u.name }
func (u *mockUser) GetAPIKey() string       { return "" }
func (u *mockUser) GetAccessToken() string  { return "" }
func (u *mockUser) GetRefreshToken() string { return "" }
func (u *mockUser) Roles() []string         { return []string{} }
func (u *mockUser) HasPermission(gimlet.PermissionOpts) bool {
	return true
}

// TODO: avoid using this global variable as a state check.
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

func mockClearUserToken(u gimlet.User, all bool) error {
	return nil
}

func mockGetUserByID(id string) (gimlet.User, bool, error) {
	opts, err := gimlet.NewBasicUserOptions(id)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	u := gimlet.NewBasicUser(opts)
	return u, true, nil
}

func mockGetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	opts, err := gimlet.NewBasicUserOptions(user.Username())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	u := gimlet.NewBasicUser(opts.Name(user.DisplayName()).Email(user.Email()).Key(user.GetAPIKey()))
	return u, nil
}

func (s *LDAPSuite) TestLDAPConstructorRequiresNonEmptyArgs() {
	l, err := NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.NoError(err)
	s.NotNil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.NoError(err)
	s.NotNil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: nil,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  nil,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  nil,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     nil,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:         "url",
		Port:        "port",
		UserPath:    "path",
		ServicePath: "bots",
		UserGroup:   "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: nil,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:                 "url",
		Port:                "port",
		ServiceUserName:     "service_user",
		ServiceUserPassword: "service_password",
		ServiceUserPath:     "service_path",
		UserPath:            "path",
		ServicePath:         "bots",
		UserGroup:           "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.NoError(err)
	s.NotNil(l)

	l, err = NewUserService(CreationOpts{
		URL:                 "url",
		Port:                "port",
		ServiceUserName:     "",
		ServiceUserPassword: "service_password",
		ServiceUserPath:     "service_path",
		UserPath:            "path",
		ServicePath:         "bots",
		UserGroup:           "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:                 "url",
		Port:                "port",
		ServiceUserName:     "service_user",
		ServiceUserPassword: "",
		ServiceUserPath:     "service_path",
		UserPath:            "path",
		ServicePath:         "bots",
		UserGroup:           "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)

	l, err = NewUserService(CreationOpts{
		URL:                 "url",
		Port:                "port",
		ServiceUserName:     "service_user",
		ServiceUserPassword: "service_password",
		ServiceUserPath:     "",
		UserPath:            "path",
		ServicePath:         "bots",
		UserGroup:           "group",
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: mockPutSuccess,
			GetUserByToken:  mockGetValid,
			ClearUserToken:  mockClearUserToken,
			GetUserByID:     mockGetUserByID,
			GetOrCreateUser: mockGetOrCreateUser,
		},
	})
	s.Error(err)
	s.Nil(l)
}

func (s *LDAPSuite) TestGetUserByToken() {
	ctx := context.Background()

	setCacheGetUserByToken := func(um gimlet.UserManager, getUserByToken usercache.GetUserByToken) {
		impl, ok := um.(*userService).cache.(*usercache.ExternalCache)
		s.Require().True(ok)
		impl.Opts.GetUserByToken = getUserByToken
	}

	setCacheGetUserByToken(s.um, mockGetErr)
	u, err := s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.serviceUserUm, mockGetErr)
	u, err = s.serviceUserUm.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.um, mockGetValid)
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByToken(s.serviceUserUm, mockGetValid)
	u, err = s.serviceUserUm.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByToken(s.um, mockGetExpired)
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByToken(s.serviceUserUm, mockGetExpired)
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByToken(s.serviceGroupUm, mockGetExpired)
	u, err = s.badGroupUm.GetUserByToken(ctx, "foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByToken(s.badGroupUm, mockGetExpired)
	u, err = s.badGroupUm.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.um, mockGetExpired)
	u, err = s.um.GetUserByToken(ctx, "badUser")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.serviceUserUm, mockGetExpired)
	u, err = s.um.GetUserByToken(ctx, "badUser")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.um, mockGetMissing)
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.serviceUserUm, mockGetMissing)
	u, err = s.um.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByToken(s.realConnUm, mockGetExpired)
	u, err = s.realConnUm.GetUserByToken(ctx, "foo")
	s.Error(err)
	s.Nil(u)
}

func (s *LDAPSuite) TestCreateUserToken() {
	_, err := s.um.CreateUserToken("foo", "badpassword")
	s.Error(err)

	_, err = s.serviceUserUm.CreateUserToken("foo", "badpassword")
	s.Error(err)

	_, err = s.um.CreateUserToken("nosuchuser", "")
	s.Error(err)

	_, err = s.serviceUserUm.CreateUserToken("nosuchuser", "")
	s.Error(err)

	token, err := s.um.CreateUserToken("foo", "hunter2")
	s.Require().NoError(err)
	s.Equal("123456", token)
	s.Equal("foo", mockPutUser.Username())
	s.Equal("Foo Bar", mockPutUser.DisplayName())
	s.Equal("foo@example.com", mockPutUser.Email())

	mockPutUser = nil
	token, err = s.serviceUserUm.CreateUserToken("foo", "hunter2")
	s.Require().NoError(err)
	s.Equal("123456", token)
	s.Equal("foo", mockPutUser.Username())
	s.Equal("Foo Bar", mockPutUser.DisplayName())
	s.Equal("foo@example.com", mockPutUser.Email())

	token, err = s.badGroupUm.CreateUserToken("foo", "hunter2")
	s.Error(err)
	s.Empty(token)

	impl, ok := s.um.(*userService).cache.(*usercache.ExternalCache)
	s.True(ok)
	impl.Opts.PutUserGetToken = mockPutErr
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
	setCacheGetUserByID := func(um gimlet.UserManager, getUserByID usercache.GetUserByID) {
		impl, ok := um.(*userService).cache.(*usercache.ExternalCache)
		s.True(ok)
		impl.Opts.GetUserByID = getUserByID
	}

	setCacheGetUserByID(s.um, mockGetErr)
	u, err := s.um.GetUserByID("foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByID(s.serviceUserUm, mockGetErr)
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByID(s.um, mockGetValid)
	u, err = s.um.GetUserByID("foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByID(s.serviceUserUm, mockGetValid)
	u, err = s.serviceUserUm.GetUserByID("foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByID(s.um, mockGetExpired)
	u, err = s.um.GetUserByID("foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByID(s.serviceUserUm, mockGetExpired)
	u, err = s.serviceUserUm.GetUserByID("foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByID(s.serviceGroupUm, mockGetExpired)
	u, err = s.serviceGroupUm.GetUserByID("foo")
	s.NoError(err)
	s.Equal("foo", u.Username())

	setCacheGetUserByID(s.badGroupUm, mockGetExpired)
	u, err = s.badGroupUm.GetUserByID("foo")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByID(s.um, mockGetExpired)
	u, err = s.um.GetUserByID("badUser")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByID(s.serviceUserUm, mockGetExpired)
	u, err = s.um.GetUserByID("badUser")
	s.Error(err)
	s.Nil(u)

	setCacheGetUserByID(s.serviceUserUm, mockGetMissing)
	u, err = s.serviceUserUm.GetUserByID("foo")
	s.Error(err)
	s.Nil(u)

	realConnImpl, ok := s.realConnUm.(*userService).cache.(*usercache.ExternalCache)
	s.True(ok)
	realConnImpl.Opts.GetUserByID = mockGetExpired
	u, err = s.realConnUm.GetUserByID("foo")
	s.Error(err)
	s.Nil(u)
}

func (s *LDAPSuite) TestGetOrCreateUser() {
	opts, err := gimlet.NewBasicUserOptions("foo")
	s.Require().NoError(err)
	basicUser := gimlet.NewBasicUser(opts)
	user, err := s.um.GetOrCreateUser(basicUser)
	s.NoError(err)
	s.Equal("foo", user.Username())
}

func (s *LDAPSuite) TestLoginUsesValidPaths() {
	userManager, ok := s.um.(*userService)
	s.True(ok)
	s.NotNil(userManager)
	s.Error(userManager.login("foo", "hunter1"))
	s.NoError(userManager.login("foo", "hunter2"))
	s.Error(userManager.login("service_user", "service_password"))
	s.NoError(userManager.loginServiceUserIfNeeded())

	serviceUserUm, ok := s.serviceUserUm.(*userService)
	s.Require().True(ok)
	s.Error(serviceUserUm.login("foo", "hunter1"))
	s.NoError(serviceUserUm.login("foo", "hunter2"))
	s.Error(serviceUserUm.login("service_user", "service_password"))
	s.NoError(serviceUserUm.loginServiceUserIfNeeded())
}

func (s *LDAPSuite) TestClearUserToken() {
	opts, err := gimlet.NewBasicUserOptions("foo")
	s.Require().NoError(err)
	basicUser := gimlet.NewBasicUser(opts)
	user, err := s.um.GetOrCreateUser(basicUser)
	s.Require().NoError(err)

	err = s.um.ClearUser(user, false)
	s.NoError(err)
}

func (s *LDAPSuite) TestGetGroupsForUser() {
	groups, err := s.um.GetGroupsForUser("foo")
	s.Require().NoError(err)
	s.Equal("10gen", groups[0])
}
