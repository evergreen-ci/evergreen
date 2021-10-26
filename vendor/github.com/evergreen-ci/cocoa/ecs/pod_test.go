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

func TestBasicECSPod(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPod)(nil), &BasicECSPod{})

	testutil.CheckAWSEnvVarsForECSAndSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, c cocoa.ECSClient){
		"InvalidPodOptions": func(ctx context.Context, t *testing.T, c cocoa.ECSClient) {
			opts := NewBasicECSPodOptions()
			p, err := NewBasicECSPod(opts)
			require.Error(t, err)
			require.Zero(t, p)
		},
		"InfoIsPopulated": func(ctx context.Context, t *testing.T, c cocoa.ECSClient) {
			res := cocoa.NewECSPodResources().SetTaskID("task_id")
			ps := cocoa.NewECSPodStatusInfo().
				SetStatus(cocoa.StatusRunning).
				AddContainers(*cocoa.NewECSContainerStatusInfo().
					SetContainerID("container_id").
					SetName("name").
					SetStatus(cocoa.StatusRunning))
			opts := NewBasicECSPodOptions().SetClient(c).SetResources(*res).SetStatusInfo(*ps)

			p, err := NewBasicECSPod(opts)
			require.NoError(t, err)

			podRes := p.Resources()
			assert.Equal(t, *res, podRes)
			assert.Equal(t, *ps, p.StatusInfo())
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
			defer c.Close(ctx)

			tCase(tctx, t, c)
		})
	}
}

func TestECSPod(t *testing.T) {
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

	smc, err := secret.NewBasicSecretsManagerClient(*awsOpts)
	require.NoError(t, err)
	defer func() {
		testutil.CleanupSecrets(ctx, t, smc)

		assert.NoError(t, smc.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSPodTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			v := secret.NewBasicSecretsManager(smc)

			pc, err := NewBasicECSPodCreator(c, v)
			require.NoError(t, err)

			tCase(tctx, t, pc, c, v)
		})
	}
}

func TestBasicECSPodOptions(t *testing.T) {
	t.Run("NewBasicECSPodOptions", func(t *testing.T) {
		opts := NewBasicECSPodOptions()
		require.NotZero(t, opts)
		assert.Zero(t, *opts)
	})
	t.Run("SetClient", func(t *testing.T) {
		c, err := NewBasicECSClient(*awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1"))
		require.NoError(t, err)
		opts := NewBasicECSPodOptions().SetClient(c)
		assert.Equal(t, c, opts.Client)
	})
	t.Run("SetVault", func(t *testing.T) {
		c, err := secret.NewBasicSecretsManagerClient(*awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1"))
		require.NoError(t, err)
		v := secret.NewBasicSecretsManager(c)
		opts := NewBasicECSPodOptions().SetVault(v)
		assert.Equal(t, v, opts.Vault)
	})
	t.Run("SetResources", func(t *testing.T) {
		res := cocoa.NewECSPodResources().SetTaskID("id")
		opts := NewBasicECSPodOptions().SetResources(*res)
		require.NotZero(t, opts.Resources)
		assert.Equal(t, *res, *opts.Resources)
	})
	t.Run("SetStatusInfo", func(t *testing.T) {
		ps := cocoa.NewECSPodStatusInfo().SetStatus(cocoa.StatusRunning)
		opts := NewBasicECSPodOptions().SetStatusInfo(*ps)
		require.NotNil(t, opts.StatusInfo)
		assert.Equal(t, *ps, *opts.StatusInfo)
	})
	t.Run("Validate", func(t *testing.T) {
		validResources := func() cocoa.ECSPodResources {
			return *cocoa.NewECSPodResources().
				SetTaskID("task_id").
				SetCluster("cluster").
				AddContainers(*cocoa.NewECSContainerResources().
					SetContainerID("container_id").
					SetName("container_name"))
		}
		validStatusInfo := func() cocoa.ECSPodStatusInfo {
			return *cocoa.NewECSPodStatusInfo().
				SetStatus(cocoa.StatusRunning).
				AddContainers(*cocoa.NewECSContainerStatusInfo().
					SetContainerID("container_id").
					SetName("name").
					SetStatus(cocoa.StatusRunning))
		}
		validAWSOpts := func() awsutil.ClientOptions {
			return *awsutil.NewClientOptions().
				SetCredentials(credentials.NewEnvCredentials()).
				SetRegion("us-east-1")
		}
		t.Run("FailsWithEmpty", func(t *testing.T) {
			opts := NewBasicECSPodOptions()
			assert.Error(t, opts.Validate())
		})
		t.Run("SucceedsWithAllFieldsPopulated", func(t *testing.T) {
			ecsClient, err := NewBasicECSClient(validAWSOpts())
			require.NoError(t, err)
			smClient, err := secret.NewBasicSecretsManagerClient(validAWSOpts())
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetVault(v).
				SetResources(validResources()).
				SetStatusInfo(validStatusInfo())
			assert.NoError(t, opts.Validate())
		})
		t.Run("FailsWithoutClient", func(t *testing.T) {
			smClient, err := secret.NewBasicSecretsManagerClient(validAWSOpts())
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			opts := NewBasicECSPodOptions().
				SetVault(v).
				SetResources(validResources()).
				SetStatusInfo(validStatusInfo())
			assert.Error(t, opts.Validate())
		})
		t.Run("SucceedsWithoutVault", func(t *testing.T) {
			ecsClient, err := NewBasicECSClient(validAWSOpts())
			require.NoError(t, err)
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetResources(validResources()).
				SetStatusInfo(validStatusInfo())
			assert.NoError(t, opts.Validate())
		})
		t.Run("FailsWithoutResources", func(t *testing.T) {
			ecsClient, err := NewBasicECSClient(validAWSOpts())
			require.NoError(t, err)
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetStatusInfo(validStatusInfo())
			assert.Error(t, opts.Validate())
		})
		t.Run("FailsWithBadResources", func(t *testing.T) {
			ecsClient, err := NewBasicECSClient(validAWSOpts())
			require.NoError(t, err)
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetResources(*cocoa.NewECSPodResources()).
				SetStatusInfo(validStatusInfo())
			assert.Error(t, opts.Validate())
		})
		t.Run("FailsWithoutStatus", func(t *testing.T) {
			ecsClient, err := NewBasicECSClient(validAWSOpts())
			require.NoError(t, err)
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetResources(validResources())
			assert.Error(t, opts.Validate())
		})
		t.Run("FailsWithBadStatus", func(t *testing.T) {
			ecsClient, err := NewBasicECSClient(validAWSOpts())
			require.NoError(t, err)
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetResources(validResources()).
				SetStatusInfo(*cocoa.NewECSPodStatusInfo())
			assert.Error(t, opts.Validate())

		})
	})
}
