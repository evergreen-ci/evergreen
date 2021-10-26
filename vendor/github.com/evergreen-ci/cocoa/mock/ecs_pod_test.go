package mock

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
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

	for tName, tCase := range ecsPodTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

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
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

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
				SetNewValue(utility.RandomString()).
				SetOwned(true))
	}
	makeContainerDef := func(t *testing.T) *cocoa.ECSContainerDefinition {
		return cocoa.NewECSContainerDefinition().
			SetImage("image").
			SetMemoryMB(128).
			SetCPU(128).
			SetName("container").
			SetCommand([]string{"echo"})
	}

	makePodCreationOpts := func(t *testing.T) *cocoa.ECSPodCreationOptions {
		return cocoa.NewECSPodCreationOptions().
			SetName(testutil.NewTaskDefinitionFamily(t)).
			SetMemoryMB(128).
			SetCPU(128).
			SetTaskRole(testutil.ECSTaskRole()).
			SetExecutionRole(testutil.ECSExecutionRole()).
			SetExecutionOptions(*cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName()))
	}

	checkPodDeleted := func(ctx context.Context, t *testing.T, p cocoa.ECSPod, c cocoa.ECSClient, smc cocoa.SecretsManagerClient, opts cocoa.ECSPodCreationOptions) {
		ps := p.StatusInfo()
		assert.Equal(t, cocoa.StatusDeleted, ps.Status)

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
					SecretId: s.ID,
				})
				assert.NoError(t, err)
				_, err = smc.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
					SecretId: s.ID,
				})
				assert.Error(t, err)
			}
		}
	}

	return map[string]func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient){
		"StopIsIdempotentWhenItFails": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(*makeContainerDef(t))
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			c.StopTaskError = errors.New("fake error")

			require.Error(t, p.Stop(ctx))

			ps := p.StatusInfo()
			assert.Equal(t, cocoa.StatusStarting, ps.Status)

			c.StopTaskError = nil

			require.NoError(t, p.Stop(ctx))
			ps = p.StatusInfo()
			assert.Equal(t, cocoa.StatusStopped, ps.Status)
		},
		"DeleteIsIdempotentWhenStoppingTaskFails": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			c.StopTaskError = errors.New("fake error")

			require.Error(t, p.Delete(ctx))

			ps := p.StatusInfo()
			require.NoError(t, err)
			assert.Equal(t, cocoa.StatusStarting, ps.Status)

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
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			c.DeregisterTaskDefinitionError = errors.New("fake error")

			require.Error(t, p.Delete(ctx))

			ps := p.StatusInfo()
			require.NoError(t, err)
			assert.Equal(t, cocoa.StatusStopped, ps.Status)

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
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			smc.DeleteSecretError = errors.New("fake error")

			require.Error(t, p.Delete(ctx))

			ps := p.StatusInfo()
			assert.Equal(t, cocoa.StatusStopped, ps.Status)

			smc.DeleteSecretError = nil

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, p, c, smc, *opts)
		},
		"LatestStatusInfoFailsWhenRequestErrors": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(*makeContainerDef(t))
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			c.DescribeTasksError = errors.New("fake error")

			ps, err := p.LatestStatusInfo(ctx)
			assert.Error(t, err)
			assert.Zero(t, ps)
		},
		"LatestStatusInfoFailsWhenRequestReturnsNoInfo": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(*makeContainerDef(t))
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			c.DescribeTasksOutput = &awsECS.DescribeTasksOutput{}

			ps, err := p.LatestStatusInfo(ctx)
			assert.Error(t, err)
			assert.Zero(t, ps)
		},
		"LatestStatusInfoFailsWhenRequestReturnsFailures": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, smc *SecretsManagerClient) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(*makeContainerDef(t))
			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			c.DescribeTasksOutput = &awsECS.DescribeTasksOutput{
				Failures: []*awsECS.Failure{{
					Arn:    p.Resources().TaskDefinition.ID,
					Detail: aws.String("fake detail"),
					Reason: aws.String("fake reason"),
				}},
			}

			ps, err := p.LatestStatusInfo(ctx)
			assert.Error(t, err)
			assert.Zero(t, ps)
		},
	}
}
