package secret

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/awsutil"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVaultInterface(t *testing.T) {
	assert.Implements(t, (*cocoa.Vault)(nil), &BasicSecretsManager{})
}

func TestSecretsManager(t *testing.T) {
	testutil.CheckAWSEnvVarsForSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanupSecret := func(ctx context.Context, t *testing.T, v cocoa.Vault, id string) {
		if id != "" {
			require.NoError(t, v.DeleteSecret(ctx, id))
		}
	}

	for tName, tCase := range testcase.VaultTests(cleanupSecret) {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
			defer tcancel()

			hc := utility.GetHTTPClient()
			defer utility.PutHTTPClient(hc)

			c, err := NewBasicSecretsManagerClient(awsutil.ClientOptions{
				Creds:  credentials.NewEnvCredentials(),
				Region: aws.String(testutil.AWSRegion()),
				Role:   aws.String(testutil.AWSRole()),
				RetryOpts: &utility.RetryOptions{
					MaxAttempts: 5,
				},
				HTTPClient: hc,
			})
			require.NoError(t, err)
			require.NotNil(t, c)

			m := NewBasicSecretsManager(c)
			require.NotNil(t, m)

			tCase(tctx, t, m)
		})
	}
}
