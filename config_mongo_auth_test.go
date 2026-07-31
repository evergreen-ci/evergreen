package evergreen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBSettingsValidate(t *testing.T) {
	t.Run("AWSAndOIDCShouldError", func(t *testing.T) {
		settings := DBSettings{
			AWSAuthEnabled:  true,
			OIDCAuthEnabled: true,
			OIDCTokenFile:   "/var/run/secrets/evergreen/atlas-token",
		}

		assert.Error(t, settings.Validate())
	})

	t.Run("OIDCWithoutTokenFileShouldError", func(t *testing.T) {
		settings := DBSettings{OIDCAuthEnabled: true}

		assert.Error(t, settings.Validate())
	})

	t.Run("OIDCWithTokenFileShouldSucceed", func(t *testing.T) {
		settings := DBSettings{
			OIDCAuthEnabled: true,
			OIDCTokenFile:   "/var/run/secrets/evergreen/atlas-token",
		}

		assert.NoError(t, settings.Validate())
	})
}

func TestDBSettingsMongoOptions(t *testing.T) {
	t.Run("AWSShouldConfigureAWSAuthentication", func(t *testing.T) {
		opts := (&DBSettings{AWSAuthEnabled: true}).mongoOptions("mongodb://localhost:27017")

		require.NotNil(t, opts.Auth)
		assert.Equal(t, awsAuthMechanism, opts.Auth.AuthMechanism)
		assert.Equal(t, mongoExternalAuthSource, opts.Auth.AuthSource)
		assert.Nil(t, opts.Auth.OIDCMachineCallback)
	})

	t.Run("OIDCShouldConfigureAndRefreshTokenFileAuthentication", func(t *testing.T) {
		tokenFile := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("first-token\n"), 0o600))

		opts := (&DBSettings{
			OIDCAuthEnabled: true,
			OIDCTokenFile:   tokenFile,
		}).mongoOptions("mongodb://localhost:27017")

		require.NotNil(t, opts.Auth)
		assert.Equal(t, oidcAuthMechanism, opts.Auth.AuthMechanism)
		assert.Equal(t, mongoExternalAuthSource, opts.Auth.AuthSource)
		require.NotNil(t, opts.Auth.OIDCMachineCallback)

		credential, err := opts.Auth.OIDCMachineCallback(t.Context(), nil)
		require.NoError(t, err)
		assert.Equal(t, "first-token", credential.AccessToken)

		require.NoError(t, os.WriteFile(tokenFile, []byte("second-token\n"), 0o600))
		credential, err = opts.Auth.OIDCMachineCallback(t.Context(), nil)
		require.NoError(t, err)
		assert.Equal(t, "second-token", credential.AccessToken)
	})

	t.Run("OIDCShouldErrorForMissingTokenFile", func(t *testing.T) {
		opts := (&DBSettings{
			OIDCAuthEnabled: true,
			OIDCTokenFile:   filepath.Join(t.TempDir(), "token"),
		}).mongoOptions("mongodb://localhost:27017")

		require.NotNil(t, opts.Auth)
		credential, err := opts.Auth.OIDCMachineCallback(t.Context(), nil)
		assert.Nil(t, credential)
		assert.Error(t, err)
	})
}
