package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestLoadUserManager(t *testing.T) {
	l := evergreen.LDAPConfig{
		URL:                "url",
		Port:               "port",
		UserPath:           "path",
		ServicePath:        "bot",
		Group:              "group",
		ExpireAfterMinutes: "60",
	}
	g := evergreen.GithubAuthConfig{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
	}
	n := evergreen.NaiveAuthConfig{}

	a := evergreen.AuthConfig{}
	um, canClearTokens, err := LoadUserManager(a)
	assert.Error(t, err, "a UserManager should not be able to be created in an empty AuthConfig")
	assert.False(t, canClearTokens)
	assert.Nil(t, um, "a UserManager should not be able to be created in an empty AuthConfig")

	a = evergreen.AuthConfig{Github: &g}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err, "a UserManager should be able to be created if one AuthConfig type is Github")
	assert.False(t, canClearTokens)
	assert.NotNil(t, um, "a UserManager should be able to be created if one AuthConfig type is Github")

	a = evergreen.AuthConfig{LDAP: &l}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err, "a UserManager should be able to be created if one AuthConfig type is LDAP")
	assert.True(t, canClearTokens)
	assert.NotNil(t, um, "a UserManager should be able to be created if one AuthConfig type is LDAP")

	a = evergreen.AuthConfig{Naive: &n}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err, "a UserManager should be able to be created if one AuthConfig type is Naive")
	assert.False(t, canClearTokens)
	assert.NotNil(t, um, "a UserManager should be able to be created if one AuthConfig type is Naive")

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthLDAPKey, LDAP: &l}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err)
	assert.True(t, canClearTokens)
	assert.NotNil(t, um)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthGithubKey, Github: &g}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err)
	assert.False(t, canClearTokens)
	assert.NotNil(t, um)
	_, ok := um.(*GithubUserManager)
	assert.True(t, ok)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthNaiveKey, Naive: &n}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err)
	assert.False(t, canClearTokens)
	assert.NotNil(t, um)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthGithubKey, LDAP: &l, Github: &g, Naive: &n}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err)
	assert.False(t, canClearTokens)
	assert.NotNil(t, um)
	_, ok = um.(*GithubUserManager)
	assert.True(t, ok)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthGithubKey, Naive: &n}
	um, canClearTokens, err = LoadUserManager(a)
	assert.NoError(t, err)
	assert.False(t, canClearTokens)
	assert.NotNil(t, um)
	_, ok = um.(*NaiveUserManager)
	assert.True(t, ok)
}

func TestSuperUserValidation(t *testing.T) {
	assert := assert.New(t)
	superUsers := []string{"super"}
	su := &simpleUser{
		UserId: "super",
	}
	ru := &simpleUser{
		UserId: "regular",
	}
	assert.True(IsSuperUser(superUsers, su))
	assert.False(IsSuperUser(superUsers, ru))
	assert.False(IsSuperUser(superUsers, nil))
}
