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
	assert.Implements((*User)(nil), NewBasicUser("", "", "", "", "", "", "", []string{}, false, nil))
	assert.Equal(MakeBasicUser(), NewBasicUser("", "", "", "", "", "", "", nil, false, nil))

	var usr *basicUser

	// accessors
	usr = &basicUser{
		ID:           "usrid",
		EmailAddress: "usr@example.net",
		Key:          "sekret",
		AccessToken:  "access_token",
		RefreshToken: "refresh_token",
		AccessRoles:  []string{"admin"},
		Name:         "name",
	}

	assert.Equal(usr.Username(), "usrid")
	assert.Equal(usr.Email(), "usr@example.net")
	assert.Equal(usr.GetAPIKey(), "sekret")
	assert.Equal(usr.GetAccessToken(), "access_token")
	assert.Equal(usr.GetRefreshToken(), "refresh_token")
	assert.Equal(usr.Roles()[0], "admin")
	assert.Equal(usr.DisplayName(), "name")

	assert.False(userHasRole(usr, "sudo"))
	assert.True(userHasRole(usr, "admin"))
}
