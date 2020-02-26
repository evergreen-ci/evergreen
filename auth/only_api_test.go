package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnlyAPIAuthManager(t *testing.T) {
	config := evergreen.AuthConfig{
		OnlyAPI: &evergreen.OnlyAPIAuthConfig{},
	}
	manager, info, err := LoadUserManager(&evergreen.Settings{AuthConfig: config})
	require.NoError(t, err)
	assert.False(t, info.CanClearTokens)
	assert.False(t, info.CanReauthorize)
	assert.Nil(t, manager.GetLoginHandler(""))
	assert.Nil(t, manager.GetLoginCallbackHandler())
}
