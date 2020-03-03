// Package ldap provides an LDAP authentication service.
package ldap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	ldap "gopkg.in/ldap.v3"
)

// userService provides authentication and authorization of users against an LDAP service. It
// implements the gimlet.Authenticator interface.
type userService struct {
	url                 string
	port                string
	userPath            string
	servicePath         string
	userGroup           string
	serviceGroup        string
	groupOuName         string
	serviceUserName     string
	serviceUserPassword string
	serviceUserPath     string
	cache               usercache.Cache
	convertIDIn         func(old string) (new string)
	unconvertIDOut      func(new string) (old string)
	connect             connectFunc
	conn                ldap.Client
}

// CreationOpts are options to pass to the service constructor.
type CreationOpts struct {
	URL          string // URL of the LDAP server
	Port         string // Port of the LDAP server
	UserPath     string // Path to users LDAP OU
	ServicePath  string // Path to service users LDAP OU
	UserGroup    string // LDAP userGroup to authorize users
	ServiceGroup string // LDAP serviceGroup to authorize services
	GroupOuName  string // name of the OU that lists a user's groups

	ServiceUserName     string // name of the service user for performing ismemberof
	ServiceUserPassword string // password for the service user
	ServiceUserPath     string // path to the service user

	UserCache usercache.Cache
	// Functions to produce a UserCache
	ExternalCache *usercache.ExternalOptions

	ConvertIDIn    func(old string) (new string)
	UnconvertIDOut func(new string) (old string)

	connect connectFunc // connect changes connection behavior for testing
}

type connectFunc func(url, port string) (ldap.Client, error)

// NewUserService constructs a userService. It requires a URL and Port to the LDAP server. It also
// requires a Path to user resources that can be passed to an LDAP query.
func NewUserService(opts CreationOpts) (gimlet.UserManager, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	var cache usercache.Cache
	if opts.UserCache != nil {
		cache = opts.UserCache
	} else {
		var err error
		cache, err = usercache.NewExternal(*opts.ExternalCache)
		if err != nil {
			return nil, errors.Wrap(err, "could not create user cache")
		}
	}
	u := &userService{
		cache:               cache,
		connect:             connect,
		url:                 opts.URL,
		port:                opts.Port,
		userPath:            opts.UserPath,
		servicePath:         opts.ServicePath,
		userGroup:           opts.UserGroup,
		serviceGroup:        opts.ServiceGroup,
		groupOuName:         opts.GroupOuName,
		serviceUserName:     opts.ServiceUserName,
		serviceUserPassword: opts.ServiceUserPassword,
		serviceUserPath:     opts.ServiceUserPath,
		convertIDIn:         opts.ConvertIDIn,
		unconvertIDOut:      opts.UnconvertIDOut,
	}

	// override, typically, for testing
	if opts.connect != nil {
		u.connect = opts.connect
	}

	return u, nil
}

func (opts CreationOpts) validate() error {
	catcher := grip.NewBasicCatcher()

	if opts.URL == "" || opts.UserPath == "" || opts.ServicePath == "" {
		catcher.Add(errors.Errorf("URL ('%s'), UserPath ('%s') and ServicePath ('%s') must be provided",
			opts.URL, opts.UserPath, opts.ServicePath))
	}

	if opts.ServiceUserName != "" {
		catcher.NewWhen(opts.ServiceUserPassword == "" || opts.ServiceUserPath == "", "if using service user, LDAP service user name, password, and path must be provided")
	} else {
		catcher.NewWhen(opts.ServiceUserPassword != "" || opts.ServiceUserPath != "", "if using service user, LDAP service user name, password, and path must be provided")
	}

	catcher.NewWhen(opts.UserGroup == "", "LDAP user group cannot be empty")

	if opts.UserCache == nil && opts.ExternalCache == nil {
		catcher.New("must specify user cache")
	}

	return catcher.Resolve()
}

// GetUserByToken returns a user for a given token. If the user is invalid (e.g., if the user's TTL
// has expired), it re-authorizes the user and re-puts the user in the cache.
func (u *userService) GetUserByToken(_ context.Context, token string) (gimlet.User, error) {
	user, valid, err := u.cache.Get(token)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting cached user")
	}
	if user == nil {
		return nil, errors.New("token is not present in cache")
	}
	if !valid {
		if err = u.ReauthorizeUser(user); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return user, nil
}

// ReauthorizeUser validates that the user is still authorized and updates their
// TTL.
func (u *userService) ReauthorizeUser(user gimlet.User) error {
	if err := u.validateGroup(user.Username()); err != nil {
		return errors.Wrap(err, "could not authorize user")
	}
	if _, err := u.cache.Put(user); err != nil {
		return errors.Wrap(err, "problem putting user in cache")
	}
	return nil
}

// CreateUserToken creates and returns a new user token from a username and password.
func (u *userService) CreateUserToken(username, password string) (string, error) {
	if err := u.login(username, password); err != nil {
		return "", errors.Wrapf(err, "failed to authenticate user '%s'", username)
	}
	if err := u.validateGroup(username); err != nil {
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
	token, err := u.cache.Put(user)
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
	user, valid, err := u.cache.Find(id)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting user by id")
	}
	if user == nil {
		return nil, errors.New("user is not present in db")
	}
	if !valid {
		if err = u.ReauthorizeUser(user); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return user, nil
}

// GetOrCreateUser gets a user from persistent storage or creates one.
func (u *userService) GetOrCreateUser(user gimlet.User) (gimlet.User, error) {
	return u.cache.GetOrCreate(user)
}

// Clear users from the cache, forcibly logging them out
func (u *userService) ClearUser(user gimlet.User, all bool) error {
	return u.cache.Clear(user, all)
}

// bind wraps u.conn.Bind, reconnecting if the LDAP server has closed the connection.
// https://github.com/go-ldap/ldap/issues/113
func (u *userService) bind(username, password string) error {
	if err := u.ensureConnected(); err != nil {
		return errors.Wrap(err, "problem connecting to ldap server")
	}
	start := time.Now()
	err := u.conn.Bind(username, password)
	grip.Info(message.Fields{
		"op":          "bind",
		"context":     "LDAP user service",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
	})
	if err == nil {
		return nil
	}
	conn, err := u.connect(u.url, u.port)
	if err != nil {
		return errors.Wrap(err, "could not connect to LDAP server")
	}
	u.conn = conn
	start = time.Now()
	err = u.conn.Bind(username, password)
	grip.Info(message.Fields{
		"op":          "bind",
		"context":     "LDAP user service",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
	})
	return err
}

// search wraps u.conn.Search, reconnecting if the LDAP server has closed the connection.
// https://github.com/go-ldap/ldap/issues/113
func (u *userService) search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if err := u.ensureConnected(); err != nil {
		return nil, errors.Wrap(err, "problem connecting to ldap server")
	}
	if err := u.loginServiceUserIfNeeded(); err != nil {
		return nil, errors.Wrap(err, "could not bind service account")
	}
	start := time.Now()
	s, err := u.conn.Search(searchRequest)
	grip.Info(message.Fields{
		"op":          "search",
		"context":     "LDAP user service",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
	})
	if err == nil {
		return s, nil
	}
	conn, err := u.connect(u.url, u.port)
	if err != nil {
		return nil, errors.Wrap(err, "could not connect to LDAP server")
	}
	u.conn = conn
	start = time.Now()
	s, err = u.conn.Search(searchRequest)
	grip.Info(message.Fields{
		"op":          "search",
		"context":     "LDAP user service",
		"duration_ms": int64(time.Since(start) / time.Millisecond),
	})
	return s, err
}

func (u *userService) ensureConnected() error {
	if u.conn == nil {
		conn, err := u.connect(u.url, u.port)
		if err != nil {
			return errors.Wrap(err, "could not connect to LDAP server")
		}
		u.conn = conn
	}
	return nil
}

func connect(host, port string) (ldap.Client, error) {
	fullURL := host
	if port != "" {
		fullURL += ":" + port
	}
	var conn *ldap.Conn
	var err error
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, errors.Wrapf(err, "problem parsing ldap url %s", fullURL)
	}
	if parsedURL.Scheme != "" && parsedURL.Scheme != host {
		conn, err = ldap.DialURL(fullURL)
	} else {
		tlsConfig := &tls.Config{ServerName: host}
		conn, err = ldap.DialTLS("tcp", fullURL, tlsConfig)
	}
	return conn, errors.Wrapf(err, "problem connecting to ldap server %s", fullURL)
}

func (u *userService) login(username, password string) error {
	var err error
	for _, path := range []string{u.userPath, u.servicePath} {
		fullPath := fmt.Sprintf("uid=%s,%s", username, path)
		err = u.bind(fullPath, password)
		if err == nil {
			return nil
		}
	}
	return errors.Wrapf(err, "could not validate user '%s'", username)
}

func (u *userService) useServiceUser() bool {
	return u.serviceUserName != "" && u.serviceUserPassword != "" && u.serviceUserPath != ""
}

// loginServiceUserIfNeeded logs in the service user if it is being used.
func (u *userService) loginServiceUserIfNeeded() error {
	if !u.useServiceUser() {
		return nil
	}
	fullPath := fmt.Sprintf("uid=%s,%s", u.serviceUserName, u.serviceUserPath)
	return errors.Wrapf(u.bind(fullPath, u.serviceUserPassword), "could not validate service user %s", u.serviceUserName)
}

// GetGroupsForUser returns the groups to which a user belongs, defined by a given cn and search path
func (u *userService) GetGroupsForUser(username string) ([]string, error) {
	groups := []string{}
	for _, path := range u.getSearchPaths() {
		if path == "" {
			return nil, errors.New("path is not specified")
		}

		searchResults, err := u.search(
			ldap.NewSearchRequest(
				path,
				ldap.ScopeWholeSubtree,
				ldap.NeverDerefAliases,
				0,
				0,
				false,
				fmt.Sprintf("(uid=%s)", username),
				[]string{"ismemberof"},
				nil))
		if err != nil {
			return nil, errors.Wrap(err, "problem searching ldap")
		}

		for _, result := range searchResults.Entries {
			for _, attr := range result.Attributes {
				for _, dnString := range attr.Values {
					groups = append(groups, findCnsWithGroup(dnString, u.groupOuName)...)
				}
			}
		}
	}

	return groups, nil
}

func findCnsWithGroup(dnString, ouName string) []string {
	cns := []string{}
	dn, err := ldap.ParseDN(dnString)
	if err != nil {
		grip.Error(errors.Wrapf(err, "error parsing %s as a valid distinguished name", dnString))
		return cns
	}
	foundGroupOu := false
	for _, v := range dn.RDNs {
		for _, field := range v.Attributes {
			if field.Type == "ou" && field.Value == ouName {
				foundGroupOu = true
			}
		}
	}
	if foundGroupOu {
		for _, v := range dn.RDNs {
			for _, field := range v.Attributes {
				if field.Type == "cn" {
					cns = append(cns, field.Value)
				}
			}
		}
	}

	return cns
}

func (u *userService) validateGroup(username string) error {
	if u.unconvertIDOut != nil {
		username = u.unconvertIDOut(username)
	}
	errs := make([]error, 0, 3)
	var (
		err    error
		result *ldap.SearchResult
	)
	for idx, path := range u.getSearchPaths() {
		if path == "" {
			errs[idx] = errors.New("path is not specified")
			continue
		}

		result, err = u.search(
			ldap.NewSearchRequest(
				path,
				ldap.ScopeWholeSubtree,
				ldap.NeverDerefAliases,
				0,
				0,
				false,
				fmt.Sprintf("(uid=%s)", username),
				[]string{"isMemberOf"},
				nil))
		if err != nil {
			errs = append(errs, errors.Wrap(err, "problem searching ldap"))
			continue
		}
		if len(result.Entries) == 0 {
			errs = append(errs, errors.Errorf("no entry returned for user '%s'", username))
			continue
		}
		if len(result.Entries[0].Attributes) == 0 {
			errs = append(errs, errors.Errorf("entry's attributes empty for user '%s'", username))
			continue
		}

		for i := range result.Entries[0].Attributes[0].Values {
			if result.Entries[0].Attributes[0].Values[i] == u.userGroup {
				return nil
			}
			if u.serviceGroup != "" && result.Entries[0].Attributes[0].Values[i] == u.serviceGroup {
				return nil
			}
		}
	}

	for _, err = range errs {
		if err != nil {
			return err
		}
	}

	return errors.Errorf("user '%s' is not a member of user group '%s' or service group '%s'", username, u.userGroup, u.serviceGroup)
}

func (u *userService) getSearchPaths() []string {
	paths := []string{u.userPath, u.servicePath}
	if u.useServiceUser() {
		paths = append(paths, u.serviceUserPath)
	}
	return paths
}

func (u *userService) getUserFromLDAP(username string) (gimlet.User, error) {
	catcher := grip.NewBasicCatcher()
	var (
		found  bool
		err    error
		result *ldap.SearchResult
	)

	for _, path := range u.getSearchPaths() {
		result, err = u.search(
			ldap.NewSearchRequest(
				path,
				ldap.ScopeWholeSubtree,
				ldap.NeverDerefAliases,
				0,
				0,
				false,
				fmt.Sprintf("(uid=%s)", username),
				[]string{},
				nil))
		if err != nil {
			catcher.Add(errors.Wrap(err, "problem searching ldap"))
			continue
		}
		if len(result.Entries) == 0 {
			catcher.Add(errors.Errorf("no entry returned for user '%s'", username))
			continue
		}
		if len(result.Entries[0].Attributes) == 0 {
			catcher.Add(errors.Errorf("entry's attributes empty for user '%s'", username))
			continue
		}

		found = true
		break
	}

	if found {
		return makeUser(result, u.convertIDIn)
	}
	return nil, catcher.Resolve()
}

func makeUser(result *ldap.SearchResult, convertIDIn func(string) string) (gimlet.User, error) {
	var (
		id     string
		name   string
		email  string
		groups []string
	)

	for _, entry := range result.Entries[0].Attributes {
		if entry.Name == "uid" {
			id = entry.Values[0]
			if convertIDIn != nil {
				id = convertIDIn(id)
			}
		}
		if entry.Name == "cn" {
			name = entry.Values[0]
		}
		if entry.Name == "mail" {
			email = entry.Values[0]
		}
		if entry.Name == "group" {
			groups = append(groups, entry.Values...)
		}
	}
	opts, err := gimlet.NewBasicUserOptions(id)
	if err != nil {
		return nil, errors.Wrap(err, "could not create user")
	}
	return gimlet.NewBasicUser(opts.Name(name).Email(email).Roles(groups...)), nil
}
