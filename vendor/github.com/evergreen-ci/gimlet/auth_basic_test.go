package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicAuthenticator(t *testing.T) {
	assert := assert.New(t)

	assert.Implements((*Authenticator)(nil), &basicAuthenticator{})
	auth := NewBasicAuthenticator([]User{}, map[string][]string{})
	assert.NotNil(auth)
	assert.NotNil(auth.(*basicAuthenticator).groups)
	assert.NotNil(auth.(*basicAuthenticator).users)
	assert.Len(auth.(*basicAuthenticator).groups, 0)
	assert.Len(auth.(*basicAuthenticator).users, 0)

	// constructor avoids nils
	auth = NewBasicAuthenticator(nil, nil)
	assert.NotNil(auth)
	assert.NotNil(auth.(*basicAuthenticator).groups)
	assert.NotNil(auth.(*basicAuthenticator).users)
	assert.Len(auth.(*basicAuthenticator).groups, 0)
	assert.Len(auth.(*basicAuthenticator).users, 0)

	// constructor avoids nils
	usr := NewBasicUser("id", "email", "key", []string{})
	auth = NewBasicAuthenticator([]User{usr}, nil)
	assert.NotNil(auth)
	assert.NotNil(auth.(*basicAuthenticator).groups)
	assert.NotNil(auth.(*basicAuthenticator).users)
	assert.Len(auth.(*basicAuthenticator).groups, 0)
	assert.Len(auth.(*basicAuthenticator).users, 1)

	// if a user exists then it should work
	assert.True(auth.CheckAuthenticated(usr))

	// a second user shouldn't validate
	usr2 := NewBasicUser("id2", "email", "key", []string{})
	assert.False(auth.CheckAuthenticated(usr2))

	usr3 := NewBasicUser("id3", "email", "key", []string{"admin"})
	usr3broken := NewBasicUser("id3", "email", "yek", []string{"admin"})
	auth = NewBasicAuthenticator([]User{usr3}, map[string][]string{
		"none":  []string{"_"},
		"admin": []string{"id3"}})
	assert.NotNil(auth)
	assert.Len(auth.(*basicAuthenticator).groups, 2)
	assert.Len(auth.(*basicAuthenticator).users, 1)
	assert.False(auth.CheckGroupAccess(usr, "admin"))
	assert.False(auth.CheckGroupAccess(usr3broken, "admin"))
	assert.False(auth.CheckGroupAccess(usr3, "proj"))
	assert.False(auth.CheckGroupAccess(usr3, "none"))
	assert.True(auth.CheckGroupAccess(usr3, "admin"))

	// check user-based role access
	usr.(*basicUser).AccessRoles = []string{"admin", "project", "one"}
	assert.False(auth.CheckResourceAccess(usr, "admin")) // not currently authenticated
	auth.(*basicAuthenticator).users[usr.Username()] = usr
	assert.True(auth.CheckResourceAccess(usr, "admin")) // now it's defined
	assert.True(auth.CheckResourceAccess(usr, "project"))
	assert.False(auth.CheckResourceAccess(usr, "two")) // but not for this role
}
