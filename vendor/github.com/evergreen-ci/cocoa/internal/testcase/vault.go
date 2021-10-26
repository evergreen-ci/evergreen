package testcase

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// VaultTestCase represents a test case for a cocoa.Vault.
type VaultTestCase func(ctx context.Context, t *testing.T, v cocoa.Vault)

// VaultTests returns common test cases that a cocoa.Vault should support.
func VaultTests(cleanupSecret func(ctx context.Context, t *testing.T, v cocoa.Vault, id string)) map[string]VaultTestCase {
	return map[string]VaultTestCase{
		"CreateSecretSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, *cocoa.NewNamedSecret().SetName(testutil.NewSecretName(t)).SetValue("hello"))
			require.NoError(t, err)
			require.NotZero(t, id)

			defer cleanupSecret(ctx, t, v, id)
		},
		"CreateSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, cocoa.NamedSecret{})
			assert.Error(t, err)
			assert.Zero(t, id)
		},
		"DeleteSecretWithExistingSecretSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, *cocoa.NewNamedSecret().SetName(testutil.NewSecretName(t)).SetValue("hello"))
			require.NoError(t, err)
			require.NotZero(t, id)

			require.NoError(t, v.DeleteSecret(ctx, id))
		},
		"DeleteSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.Error(t, v.DeleteSecret(ctx, ""))
		},
		"DeleteSecretWithValidNonexistentInputNoops": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.NoError(t, v.DeleteSecret(ctx, testutil.NewSecretName(t)))
		},
		"GetValueWithExistingSecretSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			val := "eggs"
			id, err := v.CreateSecret(ctx, *cocoa.NewNamedSecret().SetName(testutil.NewSecretName(t)).SetValue(val))
			require.NoError(t, err)
			require.NotZero(t, id)

			defer cleanupSecret(ctx, t, v, id)

			storedVal, err := v.GetValue(ctx, id)
			require.NoError(t, err)
			require.NotZero(t, val)
			assert.Equal(t, val, storedVal)
		},
		"GetValueFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.GetValue(ctx, "")
			assert.Error(t, err)
			assert.Zero(t, id)
		},
		"GetValueWithValidNonexistentInputFails": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.GetValue(ctx, testutil.NewSecretName(t))
			assert.Error(t, err)
			assert.Zero(t, id)
		},
		"UpdateValueSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, *cocoa.NewNamedSecret().SetName(testutil.NewSecretName(t)).SetValue("eggs"))
			require.NoError(t, err)
			require.NotZero(t, id)

			defer cleanupSecret(ctx, t, v, id)

			updatedVal := "ham"
			require.NoError(t, v.UpdateValue(ctx, *cocoa.NewNamedSecret().SetName(id).SetValue(updatedVal)))

			val, err := v.GetValue(ctx, id)
			require.NoError(t, err)
			require.NotZero(t, val)
			assert.Equal(t, updatedVal, val)
		},
		"UpdateValueFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.Error(t, v.UpdateValue(ctx, *cocoa.NewNamedSecret()))
		},
		"UpdateValueWithValidNonexistentInputFails": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.Error(t, v.UpdateValue(ctx, *cocoa.NewNamedSecret().SetName(testutil.NewSecretName(t)).SetValue("leaf")))
		},
	}
}
