package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLDAPConstructorRequiresNonEmptyArgs(t *testing.T) {
	assert := assert.New(t)

	l, err := newLDAPAuthenticator("foo", "bar", "baz")
	assert.NotNil(l)
	assert.NoError(err)

	l, err = newLDAPAuthenticator("", "bar", "baz")
	assert.Nil(l)
	assert.Error(err)

	l, err = newLDAPAuthenticator("foo", "", "baz")
	assert.Nil(l)
	assert.Error(err)

	l, err = newLDAPAuthenticator("foo", "bar", "")
	assert.Nil(l)
	assert.Error(err)
}

// This test requires an LDAP server, username, and password. Uncomment to test.
//
// func TestLDAPIntegration(t *testing.T) {
// 	const (
// 		url      = ""
// 		port     = ""
// 		path     = ""
// 		username = ""
// 		password = ""
// 	)
// 	assert := assert.New(t)
// 	l, err := newLDAPAuthenticator(url, port, path)
// 	assert.NotNil(l)
// 	assert.NoError(err)
// 	assert.NoError(l.connect())
// 	assert.NoError(l.login(username, password))
// }
