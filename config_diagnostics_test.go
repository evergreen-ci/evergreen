package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticsConfigValidateAndDefault(t *testing.T) {
	t.Run("EmptyConfigIsValid", func(t *testing.T) {
		c := DiagnosticsConfig{}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("FullyConfiguredIsValid", func(t *testing.T) {
		c := DiagnosticsConfig{
			S3BucketName: "my-bucket",
			S3Prefix:     "pprof",
		}
		assert.NoError(t, c.ValidateAndDefault())
	})
}

func TestDiagnosticsConfigSetAndGet(t *testing.T) {
	t.Run("SetAndGetAllFieldsSucceeds", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		}()

		original := DiagnosticsConfig{
			S3BucketName: "diagnostics-bucket",
			S3Prefix:     "pprof/prod",
		}
		require.NoError(t, original.Set(t.Context()))

		retrieved := DiagnosticsConfig{}
		require.NoError(t, retrieved.Get(t.Context()))

		assert.Equal(t, original.S3BucketName, retrieved.S3BucketName)
		assert.Equal(t, original.S3Prefix, retrieved.S3Prefix)
	})

	t.Run("SetAndGetEmptyFieldsSucceeds", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(t.Context()))
		}()

		original := DiagnosticsConfig{}
		require.NoError(t, original.Set(t.Context()))

		retrieved := DiagnosticsConfig{}
		require.NoError(t, retrieved.Get(t.Context()))

		assert.Empty(t, retrieved.S3BucketName)
		assert.Empty(t, retrieved.S3Prefix)
	})
}
