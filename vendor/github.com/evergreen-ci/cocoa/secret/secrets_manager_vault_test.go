package secret

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretsManager(t *testing.T) {
	assert.Implements(t, (*cocoa.Vault)(nil), &BasicSecretsManager{})

	testutil.CheckAWSEnvVarsForSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanupSecret := func(ctx context.Context, t *testing.T, v cocoa.Vault, id string) {
		if id != "" {
			require.NoError(t, v.DeleteSecret(ctx, id))
		}
	}

	hc := utility.GetHTTPClient()
	defer utility.PutHTTPClient(hc)

	c, err := NewBasicSecretsManagerClient(*awsutil.NewClientOptions().
		SetHTTPClient(hc).
		SetCredentials(credentials.NewEnvCredentials()).
		SetRole(testutil.AWSRole()).
		SetRegion(testutil.AWSRegion()))
	require.NoError(t, err)
	defer func() {
		testutil.CleanupSecrets(ctx, t, c)

		assert.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.VaultTests(cleanupSecret) {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			m := NewBasicSecretsManager(c)
			require.NotNil(t, m)

			tCase(tctx, t, m)
		})
	}
}
