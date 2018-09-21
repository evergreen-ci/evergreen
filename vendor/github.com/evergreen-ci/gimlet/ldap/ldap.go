// Package ldap provides an LDAP authentication service.
package ldap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	ldap "gopkg.in/ldap.v2"
)

// userService provides authentication and authorization of users against an LDAP service. It
// implements the gimlet.Authenticator interface.
type userService struct {
	CreationOpts
	conn ldap.Client
}

// CreationOpts are options to pass to the service constructor.
type CreationOpts struct {
	URL           string          // URL of the LDAP server
	Port          string          // Port of the LDAP server
	Path          string          // Path to users LDAP OU
	Group         string          // LDAP group to authorize users
	PutCache      PutUserGetToken // Put user to cache
	GetCache      GetUserByToken  // Get user from cache
	GetUser       GetUserByID     // Get user from storage
	GetCreateUser GetOrCreateUser // Get or create user from storage

	connect connectFunc // connect changes connection behavior for testing
}

// PutUserGetToken is a function provided by the client to cache users. It generates, saves, and
// returns a new token. Updating the user's TTL should happen in this function.
type PutUserGetToken func(gimlet.User) (string, error)

// GetUserByToken is a function provided by the client to retrieve cached users by token.
// It returns an error if and only if there was an error retrieving the user from the cache.
// It returns (<user>, true, nil) if the user is present in the cache and is valid.
// It returns (<user>, false, nil) if the user is present in the cache but has expired.
// It returns (nil, false, nil) if the user is not present in the cache.
type GetUserByToken func(string) (gimlet.User, bool, error)

// GetUserByID is a function provided by the client to get a user from persistent storage.
type GetUserByID func(string) (gimlet.User, error)

// GetOrCreateUser is a function provided by the client to get a user from
// persistent storage, or if the user does not exist, to create and save it.
type GetOrCreateUser func(gimlet.User) (gimlet.User, error)

type connectFunc func(url, port string) (ldap.Client, error)

// NewUserService constructs a userService. It requires a URL and Port to the LDAP server. It also
// requires a Path to user resources that can be passed to an LDAP query.
func NewUserService(opts CreationOpts) (gimlet.UserManager, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	u := &userService{}
	u.CreationOpts = CreationOpts{
		URL:           opts.URL,
		Port:          opts.Port,
		Path:          opts.Path,
		Group:         opts.Group,
		PutCache:      opts.PutCache,
		GetCache:      opts.GetCache,
		GetUser:       opts.GetUser,
		GetCreateUser: opts.GetCreateUser,
	}
	if opts.connect == nil {
		u.CreationOpts.connect = connect
	} else {
		u.CreationOpts.connect = opts.connect
	}
	return u, nil
}

func (opts CreationOpts) validate() error {
	if opts.URL == "" || opts.Port == "" || opts.Path == "" {
		return errors.Errorf("URL ('%s'), Port ('%s'), and Path ('%s') must be provided", opts.URL, opts.Port, opts.Path)
	}
	if opts.Group == "" {
		return errors.New("LDAP group cannot be empty")
	}
	if opts.PutCache == nil || opts.GetCache == nil {
		return errors.New("PutCache and GetCache must not be nil")
	}
	if opts.GetUser == nil || opts.GetCreateUser == nil {
		return errors.New("GetUserByID and GetOrCreateUser must not be nil")
	}
	return nil
}

// GetUserByToken returns a user for a given token. If the user is invalid (e.g., if the user's TTL
// has expired), it re-authorizes the user and re-puts the user in the cache.
func (u *userService) GetUserByToken(_ context.Context, token string) (gimlet.User, error) {
	user, valid, err := u.GetCache(token)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting cached user")
	}
	if user == nil {
		return nil, errors.New("token is not present in cache")
	}
	if !valid {
		if err := u.authorize(user.Username()); err != nil {
			return nil, errors.Wrap(err, "could not authorize user")
		}
		if _, err := u.PutCache(user); err != nil {
			return nil, errors.Wrap(err, "problem putting user in cache")
		}
	}
	return user, nil
}

// CreateUserToken creates and returns a new user token from a username and password.
func (u *userService) CreateUserToken(username, password string) (string, error) {
	if err := u.authenticate(username, password); err != nil {
		return "", errors.Wrapf(err, "failed to authenticate user '%s'", username)
	}
	if err := u.authorize(username); err != nil {
		return "", errors.Wrapf(err, "failed to authorize user '%s'", username)
	}
	user, err := u.getUserFromLDAP(username)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get user '%s'", username)
	}
	user, err = u.GetOrCreateUser(user)
	if err != nil {
		return "", errors.Wrap(err, "problem getting or creating user")
	}
	token, err := u.PutCache(user)
	if err != nil {
		return "", errors.Wrapf(err, "failed to put user into cache '%s'", username)
	}
	return token, nil
}

// GetLoginHandler GetLoginHandler returns nil.
func (u *userService) GetLoginHandler(url string) http.HandlerFunc { return nil }

// GetLoginCallbackHandler returns nil.
func (u *userService) GetLoginCallbackHandler() http.HandlerFunc { return nil }

// IsRedirect returns false.
func (u *userService) IsRedirect() bool { return false }

// GetUserByID gets a user from persistent storage.
func (u *userService) GetUserByID(id string) (gimlet.User, error) {
	return u.GetUser(id)
}

// GetOrCreateUser gets a user from persistent storage or creates one.
func (u *userService) GetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	return u.GetCreateUser(user)
}

// authenticate returns nil if the user and password are valid, an error otherwise.
func (u *userService) authenticate(username, password string) error {
	if err := u.ensureConnected(); err != nil {
		return errors.Wrap(err, "problem connecting to ldap server")
	}
	if err := u.login(username, password); err != nil {
		return errors.Wrapf(err, "failed to authenticate user '%s'", username)
	}
	return nil
}

// authorize returns nil if the user is a member of u.Group, an error otherwise.
func (u *userService) authorize(username string) error {
	if err := u.ensureConnected(); err != nil {
		return errors.Wrap(err, "problem connecting to ldap server")
	}
	if err := u.validateGroup(username); err != nil {
		return errors.Wrapf(err, "failed to authorize user '%s'", username)
	}
	return nil
}

func (u *userService) ensureConnected() error {
	if u.conn == nil {
		conn, err := u.connect(u.URL, u.Port)
		if err != nil {
			return errors.Wrap(err, "could not connect to LDAP server")
		}
		u.conn = conn
	}
	return nil
}

func connect(url, port string) (ldap.Client, error) {
	tlsConfig := &tls.Config{ServerName: url}
	conn, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%s", url, port), tlsConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "problem connecting to ldap server %s:%s", url, port)
	}
	return conn, nil
}

func (u *userService) login(username, password string) error {
	fullPath := fmt.Sprintf("uid=%s,%s", username, u.Path)
	return errors.Wrapf(u.conn.Bind(fullPath, password), "could not validate user '%s'", username)
}

func (u *userService) validateGroup(username string) error {
	result, err := u.conn.Search(
		ldap.NewSearchRequest(
			u.Path,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0,
			0,
			false,
			fmt.Sprintf("(uid=%s)", username),
			[]string{"ismemberof"},
			nil))
	if err != nil {
		return errors.Wrap(err, "problem searching ldap")
	}
	if len(result.Entries) == 0 {
		return errors.Errorf("no entry returned for user '%s'", username)
	}
	if len(result.Entries[0].Attributes) == 0 {
		return errors.Errorf("entry's attributes empty for user '%s'", username)
	}
	for i := range result.Entries[0].Attributes[0].Values {
		if result.Entries[0].Attributes[0].Values[i] == u.Group {
			return nil
		}
	}
	return errors.Errorf("user '%s' is not a member of group '%s'", username, u.Group)
}

func (u *userService) getUserFromLDAP(username string) (gimlet.User, error) {
	result, err := u.conn.Search(
		ldap.NewSearchRequest(
			u.Path,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0,
			0,
			false,
			fmt.Sprintf("(uid=%s)", username),
			[]string{},
			nil))
	if err != nil {
		return nil, errors.Wrap(err, "problem searching ldap")
	}
	if len(result.Entries) == 0 {
		return nil, errors.Errorf("no entry returned for user '%s'", username)
	}
	if len(result.Entries[0].Attributes) == 0 {
		return nil, errors.Errorf("entry's attributes empty for user '%s'", username)
	}
	return makeUser(result), nil
}

func makeUser(result *ldap.SearchResult) gimlet.User {
	var id, name, email string
	for _, entry := range result.Entries[0].Attributes {
		if entry.Name == "uid" {
			id = entry.Values[0]
		}
		if entry.Name == "cn" {
			name = entry.Values[0]
		}
		if entry.Name == "mail" {
			email = entry.Values[0]
		}
	}
	return gimlet.NewBasicUser(id, name, email, "", []string{})
}
