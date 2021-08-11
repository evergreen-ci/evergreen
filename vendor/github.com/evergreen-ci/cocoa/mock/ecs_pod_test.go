package mock

import (
	"context"
	"testing"
	"time"

	awsECS "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECSPod(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPod)(nil), &ECSPod{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	GlobalECSService.Clusters[testutil.ECSClusterName()] = ECSCluster{}

	for tName, tCase := range ecsPodTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			c := &ECSClient{}
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			smc := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, smc.Close(tctx))
			}()
			v := NewVault(secret.NewBasicSecretsManager(smc))

			pc, err := ecs.NewBasicECSPodCreator(c, v)
			require.NoError(t, err)
			mpc := NewECSPodCreator(pc)

			tCase(tctx, t, mpc, c, smc)
		})
	}

	for tName, tCase := range testcase.ECSPodTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			c := &ECSClient{}
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			smc := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, smc.Close(tctx))
			}()
			v := NewVault(secret.NewBasicSecretsManager(smc))

			pc, err := ecs.NewBasicECSPodCreator(c, v)
			require.NoError(t, err)
			mpc := NewECSPodCreator(pc)

			tCase(tctx, t, mpc, c, v)
		})
	}
}

// ecsPodTests are mock-specific tests for ECS and Secrets Manager with ECS
// pods.
func ecsPodTests() map[string]func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
	makeSecretEnvVar := func(t *testing.T) *cocoa.EnvironmentVariable {
		return cocoa.NewEnvironmentVariable().
			SetName(t.Name()).
			SetSecretOptions(*cocoa.NewSecretOptions().
				SetName(t.Name()).
				SetValue(utility.RandomString()).
				SetOwned(true))
	}
	makeContainerDef := func(t *testing.T) *cocoa.ECSContainerDefinition {
		return cocoa.NewECSContainerDefinition().
			SetImage("image").
			SetMemoryMB(128).
			SetCPU(128).
			SetName("container")
	}

	makePodCreationOpts := func(t *testing.T) *cocoa.ECSPodCreationOptions {
		return cocoa.NewECSPodCreationOptions().
			SetName(testutil.NewTaskDefinitionFamily(t.Name())).
			SetMemoryMB(128).
			SetCPU(128).
			SetTaskRole(testutil.TaskRole()).
			SetExecutionRole(testutil.ExecutionRole()).
			SetExecutionOptions(*cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName()))
	}

	checkPodDeleted := func(ctx context.Context, t *testing.T, p cocoa.ECSPod, c cocoa.ECSClient, smc cocoa.SecretsManagerClient, opts cocoa.ECSPodCreationOptions) {
		stat := p.StatusInfo()
		assert.Equal(t, cocoa.StatusDeleted, stat.Status)

		res := p.Resources()

		describeTaskDef, err := c.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
			TaskDefinition: res.TaskDefinition.ID,
		})
		require.NoError(t, err)
		require.NotZero(t, describeTaskDef.TaskDefinition)
		assert.Equal(t, utility.FromStringPtr(opts.Name), utility.FromStringPtr(describeTaskDef.TaskDefinition.Family))

		describeTasks, err := c.DescribeTasks(ctx, &awsECS.DescribeTasksInput{
			Cluster: res.Cluster,
			Tasks:   []*string{res.TaskID},
		})
		require.NoError(t, err)
		assert.Empty(t, describeTasks.Failures)
		require.Len(t, describeTasks.Tasks, 1)
		assert.Equal(t, awsECS.DesiredStatusStopped, utility.FromStringPtr(describeTasks.Tasks[0].LastStatus))

		for _, containerRes := range res.Containers {
			for _, s := range containerRes.Secrets {
				_, err := smc.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
					SecretId: s.Name,
				})
				assert.NoError(t, err)
				_, err = smc.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
					SecretId: s.Name,
				})
				assert.Error(t, err)
			}
		}
	}

	return map[string]func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient){
		"StopIsIdempotentWhenItFails": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(*makeContainerDef(t))
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			c.StopTaskError = errors.New("fake error")

			require.Error(t, p.Stop(ctx))

			stat := p.StatusInfo()
			assert.Equal(t, cocoa.StatusStarting, stat.Status)

			c.StopTaskError = nil

			require.NoError(t, p.Stop(ctx))
			stat = p.StatusInfo()
			assert.Equal(t, cocoa.StatusStopped, stat.Status)
		},
		"DeleteIsIdempotentWhenStoppingTaskFails": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			c.StopTaskError = errors.New("fake error")

			require.Error(t, p.Delete(ctx))

			stat := p.StatusInfo()
			require.NoError(t, err)
			assert.Equal(t, cocoa.StatusStarting, stat.Status)

			c.StopTaskError = nil

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, p, c, smc, *opts)
		},
		"DeleteIsIdempotentWhenDeregisteringTaskDefinitionFails": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			c.DeregisterTaskDefinitionError = errors.New("fake error")

			require.Error(t, p.Delete(ctx))

			stat := p.StatusInfo()
			require.NoError(t, err)
			assert.Equal(t, cocoa.StatusStopped, stat.Status)

			c.DeregisterTaskDefinitionError = nil

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, p, c, smc, *opts)
		},
		"DeleteIsIdempotentWhenDeletingSecretsFails": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			smc.DeleteSecretError = errors.New("fake error")

			require.Error(t, p.Delete(ctx))

			stat := p.StatusInfo()
			assert.Equal(t, cocoa.StatusStopped, stat.Status)

			smc.DeleteSecretError = nil

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, p, c, smc, *opts)
		},
	}
}
