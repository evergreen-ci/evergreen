package testcase

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
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
		"CreateSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, cocoa.NamedSecret{})
			assert.Error(t, err)
			assert.Zero(t, id)
		},
		"DeleteSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.Error(t, v.DeleteSecret(ctx, ""))
		},
		"DeleteSecretWithExistingSecretSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			out, err := v.CreateSecret(ctx, cocoa.NamedSecret{
				Name:  aws.String(testutil.NewSecretName(t.Name())),
				Value: aws.String("hello")})

			require.NoError(t, err)
			require.NotZero(t, out)

			defer cleanupSecret(ctx, t, v, out)
		},
		"GetValueFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			out, err := v.GetValue(ctx, "")
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"UpdateFailsWithInvalidInput": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.Error(t, v.UpdateValue(ctx, *cocoa.NewNamedSecret().SetName("").SetValue("")))
		},
		"GetValueWithExistingSecretSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, cocoa.NamedSecret{
				Name:  aws.String(testutil.NewSecretName(t.Name())),
				Value: aws.String("eggs")})

			require.NoError(t, err)
			require.NotZero(t, id)

			defer cleanupSecret(ctx, t, v, id)

			val, err := v.GetValue(ctx, id)
			require.NoError(t, err)
			require.NotZero(t, val)
			assert.Equal(t, "eggs", val)
		},
		"UpdateValueSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			id, err := v.CreateSecret(ctx, cocoa.NamedSecret{
				Name:  aws.String(testutil.NewSecretName(t.Name())),
				Value: aws.String("eggs"),
			})
			require.NoError(t, err)
			require.NotZero(t, id)

			defer cleanupSecret(ctx, t, v, id)

			require.NoError(t, v.UpdateValue(ctx, *cocoa.NewNamedSecret().SetName(id).SetValue("ham")))

			val, err := v.GetValue(ctx, id)
			require.NoError(t, err)
			require.NotZero(t, val)
			assert.Equal(t, "ham", val)
		},
		"DeleteSecretWithValidNonexistentInputWillNoop": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.NoError(t, v.DeleteSecret(ctx, testutil.NewSecretName(t.Name())))
		},
		"GetValueWithValidNonexistentInputFails": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			out, err := v.GetValue(ctx, testutil.NewSecretName(t.Name()))
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"UpdateValueWithValidNonexistentInputFails": func(ctx context.Context, t *testing.T, v cocoa.Vault) {
			assert.Error(t, v.UpdateValue(ctx, *cocoa.NewNamedSecret().SetName(testutil.NewSecretName(t.Name())).SetValue("leaf")))
		},
	}
}
