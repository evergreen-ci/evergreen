package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitHubRecoveryAppConfigValidateAndDefault(t *testing.T) {
	t.Run("EmptyConfigIsValid", func(t *testing.T) {
		c := GitHubRecoveryAppConfig{}
		assert.NoError(t, c.ValidateAndDefault())
	})
	t.Run("AppIDWithPrivateKeyIsValid", func(t *testing.T) {
		c := GitHubRecoveryAppConfig{AppID: 1234, PrivateKey: "private_key"}
		assert.NoError(t, c.ValidateAndDefault())
	})
	t.Run("AppIDWithoutPrivateKeyShouldError", func(t *testing.T) {
		c := GitHubRecoveryAppConfig{AppID: 1234}
		assert.Error(t, c.ValidateAndDefault())
	})
	t.Run("PrivateKeyWithoutAppIDIsValid", func(t *testing.T) {
		// Clearing a secret leaves its old value in Parameter Store, so a key
		// can still be read back after an admin clears the app ID.
		c := GitHubRecoveryAppConfig{PrivateKey: "private_key"}
		assert.NoError(t, c.ValidateAndDefault())
	})
}
