package gimlet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicUserImplementation(t *testing.T) {
	assert := assert.New(t)

	// constructors
	assert.Implements((*User)(nil), &basicUser{})
	assert.Implements((*User)(nil), MakeBasicUser())
	assert.Implements((*User)(nil), NewBasicUser("", "", "", []string{}))
	assert.Equal(MakeBasicUser(), NewBasicUser("", "", "", nil))

	var usr *basicUser
	assert.True(usr.IsNil())

	// accessors
	usr = &basicUser{
		ID:           "usrid",
		EmailAddress: "usr@example.net",
		Key:          "sekret",
		AccessRoles:  []string{"admin"},
	}

	assert.Equal(usr.Username(), "usrid")
	assert.Equal(usr.Email(), "usr@example.net")
	assert.Equal(usr.GetAPIKey(), "sekret")
	assert.Equal(usr.Roles()[0], "admin")
	assert.Contains(usr.DisplayName(), usr.ID)
	assert.Contains(usr.DisplayName(), usr.EmailAddress)
	assert.False(usr.IsNil())

	assert.False(userHasRole(usr, "sudo"))
	assert.True(userHasRole(usr, "admin"))
}
