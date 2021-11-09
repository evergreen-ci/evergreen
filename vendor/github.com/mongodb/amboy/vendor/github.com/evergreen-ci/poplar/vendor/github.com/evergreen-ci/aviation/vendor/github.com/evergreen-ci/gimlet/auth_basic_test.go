package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleAuthenticator(t *testing.T) {
	assert := assert.New(t)

	assert.Implements((*Authenticator)(nil), &simpleAuthenticator{})
	auth := NewSimpleAuthenticator([]User{}, map[string][]string{})
	assert.NotNil(auth)
	assert.NotNil(auth.(*simpleAuthenticator).groups)
	assert.NotNil(auth.(*simpleAuthenticator).users)
	assert.Len(auth.(*simpleAuthenticator).groups, 0)
	assert.Len(auth.(*simpleAuthenticator).users, 0)

	// constructor avoids nils
	auth = NewSimpleAuthenticator(nil, nil)
	assert.NotNil(auth)
	assert.NotNil(auth.(*simpleAuthenticator).groups)
	assert.NotNil(auth.(*simpleAuthenticator).users)
	assert.Len(auth.(*simpleAuthenticator).groups, 0)
	assert.Len(auth.(*simpleAuthenticator).users, 0)

	// constructor avoids nils
	usr := NewBasicUser("id", "name", "email", "key", []string{})
	auth = NewSimpleAuthenticator([]User{usr}, nil)
	assert.NotNil(auth)
	assert.NotNil(auth.(*simpleAuthenticator).groups)
	assert.NotNil(auth.(*simpleAuthenticator).users)
	assert.Len(auth.(*simpleAuthenticator).groups, 0)
	assert.Len(auth.(*simpleAuthenticator).users, 1)

	// if a user exists then it should work
	assert.True(auth.CheckAuthenticated(usr))

	// a second user shouldn't validate
	usr2 := NewBasicUser("id2", "name", "email", "key", []string{})
	assert.False(auth.CheckAuthenticated(usr2))

	usr3 := NewBasicUser("id3", "name", "email", "key", []string{"admin"})
	usr3broken := NewBasicUser("id3", "name", "email", "yek", []string{"admin"})
	auth = NewSimpleAuthenticator([]User{usr3}, map[string][]string{
		"none":  []string{"_"},
		"admin": []string{"id3"}})
	assert.NotNil(auth)
	assert.Len(auth.(*simpleAuthenticator).groups, 2)
	assert.Len(auth.(*simpleAuthenticator).users, 1)
	assert.False(auth.CheckGroupAccess(usr, "admin"))
	assert.False(auth.CheckGroupAccess(usr3broken, "admin"))
	assert.False(auth.CheckGroupAccess(usr3, "proj"))
	assert.False(auth.CheckGroupAccess(usr3, "none"))
	assert.True(auth.CheckGroupAccess(usr3, "admin"))

	// check user-based role access
	usr.(*basicUser).AccessRoles = []string{"admin", "project", "one"}
	assert.False(auth.CheckResourceAccess(usr, "admin")) // not currently authenticated
	auth.(*simpleAuthenticator).users[usr.Username()] = usr
	assert.True(auth.CheckResourceAccess(usr, "admin")) // now it's defined
	assert.True(auth.CheckResourceAccess(usr, "project"))
	assert.False(auth.CheckResourceAccess(usr, "two")) // but not for this role
}

func TestBasicAuthenticator(t *testing.T) {
	assert := assert.New(t)
	// constructor avoids nils
	auth := NewBasicAuthenticator(nil, nil)
	assert.NotNil(auth)
	assert.NotNil(auth.(*basicAuthenticator).groups)
	assert.NotNil(auth.(*basicAuthenticator).resources)
	assert.Len(auth.(*basicAuthenticator).groups, 0)
	assert.Len(auth.(*basicAuthenticator).resources, 0)

	// authenticated users are all non-nil users that have
	// usernames
	assert.False(auth.CheckAuthenticated(nil))
	assert.False(auth.CheckAuthenticated(&basicUser{}))
	usr := NewBasicUser("id", "name", "email", "key", []string{})
	assert.True(auth.CheckAuthenticated(usr))

	auth = NewBasicAuthenticator(map[string][]string{"one": []string{"id"}}, nil)
	assert.False(auth.CheckGroupAccess(usr, "two"))
	assert.False(auth.CheckGroupAccess(usr, ""))
	assert.True(auth.CheckGroupAccess(usr, "one"))
	assert.False(auth.CheckGroupAccess(nil, "one"))
	assert.False(auth.CheckGroupAccess(nil, ""))

	auth = NewBasicAuthenticator(nil, map[string][]string{"/one": []string{"id"}})
	assert.False(auth.CheckResourceAccess(usr, "two"))
	assert.False(auth.CheckResourceAccess(usr, ""))
	assert.False(auth.CheckResourceAccess(usr, "one"))
	assert.False(auth.CheckResourceAccess(usr, "/two"))
	assert.False(auth.CheckResourceAccess(usr, "/"))
	assert.True(auth.CheckResourceAccess(usr, "/one"))
	assert.False(auth.CheckResourceAccess(nil, "/one"))
	assert.False(auth.CheckResourceAccess(nil, ""))

}
