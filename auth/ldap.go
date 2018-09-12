package auth

import (
	"crypto/tls"
	"fmt"

	"github.com/pkg/errors"
	ldap "gopkg.in/ldap.v2"
)

type ldapAuthenticator struct {
	url  string
	port string
	path string
	conn *ldap.Conn //nolint
}

func newLDAPAuthenticator(url, port, path string) (*ldapAuthenticator, error) { //nolint
	if url == "" || port == "" || path == "" {
		return nil, errors.Errorf("url ('%s'), port ('%s'), and path ('%s') must be provided", url, port, path)
	}
	return &ldapAuthenticator{
		url:  url,
		port: port,
		path: path,
	}, nil
}

func (l *ldapAuthenticator) connect() error { //nolint
	tlsConfig := &tls.Config{ServerName: l.url}
	conn, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%s", l.url, l.port), tlsConfig)
	if err != nil {
		return errors.Wrapf(err, "problem connecting to ldap server %s:%s", l.url, l.port)
	}
	l.conn = conn
	return nil
}

func (l *ldapAuthenticator) login(username, password string) error { //nolint
	fullPath := fmt.Sprintf("uid=%s,%s", username, l.path)
	return errors.Wrapf(l.conn.Bind(fullPath, password), "could not validate user '%s'", username)
}
