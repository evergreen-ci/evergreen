package ecs

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
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECSPodCreatorInterface(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPodCreator)(nil), &BasicECSPodCreator{})
}

func TestBasicECSPodCreator(t *testing.T) {
	testutil.CheckAWSEnvVarsForECSAndSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, c cocoa.ECSClient, v cocoa.Vault){
		"NewPodCreatorFailsWithMissingClientAndVault": func(ctx context.Context, t *testing.T, c cocoa.ECSClient, v cocoa.Vault) {
			podCreator, err := NewBasicECSPodCreator(nil, nil)
			require.Error(t, err)
			require.Zero(t, podCreator)
		},
		"NewPodCreatorFailsWithMissingClient": func(ctx context.Context, t *testing.T, c cocoa.ECSClient, v cocoa.Vault) {
			podCreator, err := NewBasicECSPodCreator(nil, v)
			require.Error(t, err)
			require.Zero(t, podCreator)
		},
		"NewPodCreatorSucceedsWithNoVault": func(ctx context.Context, t *testing.T, c cocoa.ECSClient, v cocoa.Vault) {
			podCreator, err := NewBasicECSPodCreator(c, nil)
			require.NoError(t, err)
			require.NotZero(t, podCreator)
		},
		"NewPodCreatorSucceedsWithClientAndVault": func(ctx context.Context, t *testing.T, c cocoa.ECSClient, v cocoa.Vault) {
			podCreator, err := NewBasicECSPodCreator(c, v)
			require.NoError(t, err)
			require.NotZero(t, podCreator)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
			defer tcancel()

			hc := utility.GetHTTPClient()
			defer utility.PutHTTPClient(hc)

			awsOpts := awsutil.NewClientOptions().
				SetHTTPClient(hc).
				SetCredentials(credentials.NewEnvCredentials()).
				SetRole(testutil.AWSRole()).
				SetRegion(testutil.AWSRegion())

			c, err := NewBasicECSClient(*awsOpts)
			require.NoError(t, err)
			defer c.Close(ctx)

			secretsClient, err := secret.NewBasicSecretsManagerClient(awsutil.ClientOptions{
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
			defer secretsClient.Close(ctx)

			m := secret.NewBasicSecretsManager(secretsClient)
			require.NotNil(t, m)

			tCase(tctx, t, c, m)
		})
	}
}

func TestECSPodCreator(t *testing.T) {
	testutil.CheckAWSEnvVarsForECSAndSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range testcase.ECSPodCreatorTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
			defer tcancel()

			hc := utility.GetHTTPClient()
			defer utility.PutHTTPClient(hc)

			awsOpts := awsutil.NewClientOptions().
				SetHTTPClient(hc).
				SetCredentials(credentials.NewEnvCredentials()).
				SetRole(testutil.AWSRole()).
				SetRegion(testutil.AWSRegion())

			c, err := NewBasicECSClient(*awsOpts)
			require.NoError(t, err)
			defer c.Close(tctx)

			podCreator, err := NewBasicECSPodCreator(c, nil)
			require.NoError(t, err)

			tCase(tctx, t, podCreator)
		})
	}

	for tName, tCase := range testcase.ECSPodCreatorWithVaultTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 30*time.Second)
			defer tcancel()

			hc := utility.GetHTTPClient()
			defer utility.PutHTTPClient(hc)

			awsOpts := awsutil.NewClientOptions().
				SetHTTPClient(hc).
				SetCredentials(credentials.NewEnvCredentials()).
				SetRole(testutil.AWSRole()).
				SetRegion(testutil.AWSRegion())

			c, err := NewBasicECSClient(*awsOpts)
			require.NoError(t, err)
			defer c.Close(tctx)

			secretsClient, err := secret.NewBasicSecretsManagerClient(awsutil.ClientOptions{
				Creds:  credentials.NewEnvCredentials(),
				Region: aws.String(testutil.AWSRegion()),
				Role:   aws.String(testutil.AWSRole()),
				RetryOpts: &utility.RetryOptions{
					MaxAttempts: 5,
				},
				HTTPClient: hc,
			})
			require.NoError(t, err)
			defer secretsClient.Close(tctx)

			m := secret.NewBasicSecretsManager(secretsClient)
			require.NotNil(t, m)

			podCreator, err := NewBasicECSPodCreator(c, m)
			require.NoError(t, err)

			tCase(tctx, t, podCreator)
		})
	}
}
