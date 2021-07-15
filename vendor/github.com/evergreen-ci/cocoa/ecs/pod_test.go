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

func TestECSPodInterface(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPod)(nil), &BasicECSPod{})
}

func TestECSPodBasics(t *testing.T) {
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
			stat := cocoa.Starting
			opts := NewBasicECSPodOptions().SetClient(c).SetResources(*res).SetStatus(stat)

			p, err := NewBasicECSPod(opts)
			require.NoError(t, err)

			info, err := p.Info(ctx)
			require.NoError(t, err)
			assert.Equal(t, *res, info.Resources)
			assert.Equal(t, stat, info.Status)
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

			tCase(tctx, t, c)
		})

	}

}

// TODO (EVG-14979): clean up resources in ECS tests more thoroughly
func TestECSPod(t *testing.T) {
	testutil.CheckAWSEnvVarsForECSAndSecretsManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range testcase.ECSPodTests() {
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

			pc, err := NewBasicECSPodCreator(c, m)
			require.NoError(t, err)
			require.NotZero(t, pc)

			tCase(tctx, t, m, pc)
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
	t.Run("SetStatus", func(t *testing.T) {
		stat := cocoa.Starting
		opts := NewBasicECSPodOptions().SetStatus(stat)
		require.NotNil(t, opts.Status)
		assert.Equal(t, stat, *opts.Status)
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			opts := NewBasicECSPodOptions()
			assert.Error(t, opts.Validate())
		})
		t.Run("AllFieldsPopulatedIsValid", func(t *testing.T) {
			awsOpts := awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1")
			ecsClient, err := NewBasicECSClient(*awsOpts)
			require.NoError(t, err)
			smClient, err := secret.NewBasicSecretsManagerClient(*awsOpts)
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			res := cocoa.NewECSPodResources().SetTaskID("id")
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetVault(v).
				SetResources(*res).
				SetStatus(cocoa.Starting)
			assert.NoError(t, opts.Validate())
		})
		t.Run("MissingClientIsInvalid", func(t *testing.T) {
			awsOpts := awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1")
			smClient, err := secret.NewBasicSecretsManagerClient(*awsOpts)
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			res := cocoa.NewECSPodResources().SetTaskID("id")
			opts := NewBasicECSPodOptions().
				SetVault(v).
				SetResources(*res).
				SetStatus(cocoa.Starting)
			assert.Error(t, opts.Validate())
		})
		t.Run("MissingVaultIsValid", func(t *testing.T) {
			awsOpts := awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1")
			ecsClient, err := NewBasicECSClient(*awsOpts)
			require.NoError(t, err)
			res := cocoa.NewECSPodResources().SetTaskID("id")
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetResources(*res).
				SetStatus(cocoa.Starting)
			assert.NoError(t, opts.Validate())
		})
		t.Run("MissingResourcesIsInvalid", func(t *testing.T) {
			awsOpts := awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1")
			smClient, err := secret.NewBasicSecretsManagerClient(*awsOpts)
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			res := cocoa.NewECSPodResources()
			opts := NewBasicECSPodOptions().
				SetVault(v).
				SetResources(*res).
				SetStatus(cocoa.Starting)
			assert.Error(t, opts.Validate())
		})
		t.Run("BadResourcesIsInvalid", func(t *testing.T) {
			awsOpts := awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1")
			smClient, err := secret.NewBasicSecretsManagerClient(*awsOpts)
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			opts := NewBasicECSPodOptions().
				SetVault(v).
				SetStatus(cocoa.Starting)
			assert.Error(t, opts.Validate())
		})
		t.Run("MissingStatusIsInvalid", func(t *testing.T) {
			awsOpts := awsutil.NewClientOptions().SetCredentials(credentials.NewEnvCredentials()).SetRegion("us-east-1")
			ecsClient, err := NewBasicECSClient(*awsOpts)
			require.NoError(t, err)
			smClient, err := secret.NewBasicSecretsManagerClient(*awsOpts)
			require.NoError(t, err)
			v := secret.NewBasicSecretsManager(smClient)
			res := cocoa.NewECSPodResources().SetTaskID("id")
			opts := NewBasicECSPodOptions().
				SetClient(ecsClient).
				SetVault(v).
				SetResources(*res)
			assert.Error(t, opts.Validate())
		})
	})
}
