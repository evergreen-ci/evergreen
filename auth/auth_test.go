package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	_ "github.com/evergreen-ci/evergreen/testutil" // Import testutil to force init to run
	"github.com/stretchr/testify/assert"
)

func TestLoadUserManager(t *testing.T) {
	ldap := evergreen.LDAPConfig{
		URL:                "url",
		Port:               "port",
		UserPath:           "path",
		ServicePath:        "bot",
		Group:              "group",
		ExpireAfterMinutes: "60",
	}
	github := evergreen.GithubAuthConfig{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
	}
	naive := evergreen.NaiveAuthConfig{}
	okta := evergreen.OktaConfig{
		ClientID:     "client_id",
		ClientSecret: "client_secret",
		Issuer:       "issuer",
		UserGroup:    "user_group",
	}
	multi := evergreen.MultiAuthConfig{
		ReadWrite: []string{evergreen.AuthLDAPKey},
		ReadOnly:  []string{evergreen.AuthNaiveKey},
	}
	multiNoInfo := evergreen.MultiAuthConfig{
		ReadWrite: []string{evergreen.AuthGithubKey},
	}

	a := evergreen.AuthConfig{}
	um, info, err := LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.Error(t, err, "a UserManager should not be created in an empty AuthConfig")
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.Nil(t, um, "a UserManager should not be created in an empty AuthConfig")

	a = evergreen.AuthConfig{Github: &github}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is Github")
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is Github")

	a = evergreen.AuthConfig{LDAP: &ldap}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is LDAP")
	assert.True(t, info.CanClearTokens)
	assert.True(t, info.CanReauthorize)
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is LDAP")

	a = evergreen.AuthConfig{Okta: &okta}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is Okta")
	assert.True(t, info.CanClearTokens)
	assert.True(t, info.CanReauthorize)
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is Okta")

	a = evergreen.AuthConfig{Naive: &naive}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is Naive")
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is Naive")

	a = evergreen.AuthConfig{AllowServiceUsers: true}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is OnlyAPI")
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is OnlyAPI")

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthMultiKey, Multi: &multi, LDAP: &ldap, Naive: &naive}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is Multi")
	assert.True(t, info.CanClearTokens, "should be able to clear tokens if the underlying manager is able")
	assert.True(t, info.CanReauthorize, "should be able to reauthorize if the underlying manager is able")
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is Multi")

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthMultiKey, Multi: &multiNoInfo, Github: &github, AllowServiceUsers: true}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err, "a UserManager should be created if one AuthConfig type is Multi")
	assert.False(t, info.CanClearTokens, "should not be able to clear tokens if the underlying user managers is unable")
	assert.False(t, info.CanReauthorize, "should not be able to reauthorize if the underlying user managers is unable")
	assert.NotNil(t, um, "a UserManager should be created if one AuthConfig type is Multi")

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthLDAPKey, LDAP: &ldap}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err)
	assert.True(t, info.CanClearTokens)
	assert.True(t, info.CanReauthorize)
	assert.NotNil(t, um)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthOktaKey, Okta: &okta}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err)
	assert.True(t, info.CanClearTokens)
	assert.True(t, info.CanReauthorize)
	assert.NotNil(t, um)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthGithubKey, Github: &github}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um)
	_, ok := um.(*GithubUserManager)
	assert.True(t, ok)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthNaiveKey, Naive: &naive}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthGithubKey, LDAP: &ldap, Github: &github, Naive: &naive}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um)
	_, ok = um.(*GithubUserManager)
	assert.True(t, ok)

	a = evergreen.AuthConfig{PreferredType: evergreen.AuthGithubKey, Naive: &naive}
	um, info, err = LoadUserManager(&evergreen.Settings{AuthConfig: a})
	assert.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.NotNil(t, um)
	_, ok = um.(*NaiveUserManager)
	assert.True(t, ok)
}
