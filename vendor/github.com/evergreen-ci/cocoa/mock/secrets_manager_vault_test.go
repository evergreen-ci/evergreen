package mock

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultWithSecretsManager(t *testing.T) {
	assert.Implements(t, (*cocoa.Vault)(nil), &Vault{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanupSecret := func(ctx context.Context, t *testing.T, v cocoa.Vault, id string) {
		if id != "" {
			require.NoError(t, v.DeleteSecret(ctx, id))
		}
	}

	for tName, tCase := range testcase.VaultTests(cleanupSecret) {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

			c := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, c.Close(tctx))
			}()
			v := NewVault(secret.NewBasicSecretsManager(c))

			tCase(tctx, t, v)
		})
	}
}
