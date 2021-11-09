package secret

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTestTimeout is the standard timeout for integration tests against
// Secrets Manager.
const defaultTestTimeout = time.Minute

func TestSecretsManagerClient(t *testing.T) {
	assert.Implements(t, (*cocoa.SecretsManagerClient)(nil), &BasicSecretsManagerClient{})

	testutil.CheckAWSEnvVarsForSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	for tName, tCase := range testcase.SecretsManagerClientTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			tCase(tctx, t, c)
		})
	}

}
