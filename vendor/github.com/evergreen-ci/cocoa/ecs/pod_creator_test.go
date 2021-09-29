package ecs

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicECSPodCreator(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPodCreator)(nil), &BasicECSPodCreator{})

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
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
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
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			smc, err := secret.NewBasicSecretsManagerClient(*awsOpts)
			require.NoError(t, err)
			require.NotNil(t, c)
			defer func() {
				assert.NoError(t, smc.Close(tctx))
			}()

			m := secret.NewBasicSecretsManager(smc)
			require.NotNil(t, m)

			tCase(tctx, t, c, m)
		})
	}
}

func TestECSPodCreator(t *testing.T) {
	testutil.CheckAWSEnvVarsForECSAndSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hc := utility.GetHTTPClient()
	defer utility.PutHTTPClient(hc)

	awsOpts := awsutil.NewClientOptions().
		SetHTTPClient(hc).
		SetCredentials(credentials.NewEnvCredentials()).
		SetRole(testutil.AWSRole()).
		SetRegion(testutil.AWSRegion())

	c, err := NewBasicECSClient(*awsOpts)
	require.NoError(t, err)
	defer func() {
		testutil.CleanupTaskDefinitions(ctx, t, c)
		testutil.CleanupTasks(ctx, t, c)

		assert.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSPodCreatorTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			pc, err := NewBasicECSPodCreator(c, nil)
			require.NoError(t, err)

			tCase(tctx, t, pc)
		})
	}

	smc, err := secret.NewBasicSecretsManagerClient(*awsOpts)
	require.NoError(t, err)
	defer func() {
		testutil.CleanupSecrets(ctx, t, smc)

		assert.NoError(t, smc.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSPodCreatorWithVaultTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			m := secret.NewBasicSecretsManager(smc)
			require.NotNil(t, m)

			pc, err := NewBasicECSPodCreator(c, m)
			require.NoError(t, err)

			tCase(tctx, t, pc)
		})
	}
}
