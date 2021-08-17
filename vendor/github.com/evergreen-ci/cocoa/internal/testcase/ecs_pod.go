package testcase

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ECSPodTestCase represents a test case for a cocoa.ECSPod.
type ECSPodTestCase func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault)

// ECSPodTests returns common test cases that a cocoa.ECSPod should support.
func ECSPodTests() map[string]ECSPodTestCase {
	makeEnvVar := func(t *testing.T) *cocoa.EnvironmentVariable {
		return cocoa.NewEnvironmentVariable().
			SetName(t.Name()).
			SetValue("value")
	}

	makeSecretEnvVar := func(t *testing.T) *cocoa.EnvironmentVariable {
		return cocoa.NewEnvironmentVariable().
			SetName(t.Name()).
			SetSecretOptions(*cocoa.NewSecretOptions().
				SetName(testutil.NewSecretName(t.Name())).
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

	return map[string]ECSPodTestCase{
		"InfoIsPopulated": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			defer cleanupPod(ctx, t, p, c, v)

			stat := p.StatusInfo()
			assert.Equal(t, cocoa.StatusStarting, stat.Status)

			res := p.Resources()
			require.NoError(t, err)
			assert.NotZero(t, res.TaskID)
			assert.NotZero(t, res.TaskDefinition)
			assert.Equal(t, opts.ExecutionOpts.Cluster, res.Cluster)

			require.Len(t, res.Containers, 1)
			require.Len(t, res.Containers[0].Secrets, 1)
			for _, s := range res.Containers[0].Secrets {
				val, err := v.GetValue(ctx, utility.FromStringPtr(s.Name))
				require.NoError(t, err)
				assert.Equal(t, utility.FromStringPtr(s.Value), val)
				assert.True(t, utility.FromBoolPtr(s.Owned))
			}

			require.True(t, utility.FromBoolPtr(res.TaskDefinition.Owned))
			def, err := c.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: res.TaskDefinition.ID,
			})
			require.NoError(t, err)
			require.NotZero(t, def.TaskDefinition)
			assert.Equal(t, utility.FromStringPtr(opts.Name), utility.FromStringPtr(def.TaskDefinition.Family))

			task, err := c.DescribeTasks(ctx, &ecs.DescribeTasksInput{
				Cluster: res.Cluster,
				Tasks:   []*string{res.TaskID},
			})
			require.NoError(t, err)
			require.Len(t, task.Tasks, 1)
			require.Len(t, task.Tasks[0].Containers, 1)
			assert.Equal(t, utility.FromStringPtr(opts.ContainerDefinitions[0].Image), utility.FromStringPtr(task.Tasks[0].Containers[0].Image))
		},
		"StopSucceeds": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			defer cleanupPod(ctx, t, p, c, v)

			require.NoError(t, p.Stop(ctx))

			checkPodStatus(t, p, cocoa.StatusStopped)
		},
		"StopSucceedsWithSecrets": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			secret := cocoa.NewEnvironmentVariable().
				SetName(utility.RandomString()).
				SetSecretOptions(*cocoa.NewSecretOptions().
					SetName(testutil.NewSecretName(t.Name())).
					SetValue(utility.RandomString()))
			ownedSecret := makeSecretEnvVar(t)

			secretOpts := makePodCreationOpts(t).
				AddContainerDefinitions(*makeContainerDef(t).
					AddEnvironmentVariables(*secret, *ownedSecret))

			p, err := pc.CreatePod(ctx, secretOpts)
			require.NoError(t, err)

			defer cleanupPod(ctx, t, p, c, v)

			res := p.Resources()
			require.Len(t, res.Containers, 1)
			assert.Len(t, res.Containers[0].Secrets, 2)

			checkPodStatus(t, p, cocoa.StatusStarting)

			require.NoError(t, p.Stop(ctx))

			res = p.Resources()
			require.Len(t, res.Containers, 1)
			require.Len(t, res.Containers[0].Secrets, 2)

			val, err := v.GetValue(ctx, utility.FromStringPtr(res.Containers[0].Secrets[0].Name))
			require.NoError(t, err)
			assert.Equal(t, utility.FromStringPtr(secret.SecretOpts.Value), val)

			val, err = v.GetValue(ctx, utility.FromStringPtr(res.Containers[0].Secrets[1].Name))
			require.NoError(t, err)
			assert.Equal(t, utility.FromStringPtr(ownedSecret.SecretOpts.Value), val)

			checkPodStatus(t, p, cocoa.StatusStopped)
		},
		"StopIsIdempotent": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			defer cleanupPod(ctx, t, p, c, v)

			require.NoError(t, p.Stop(ctx))

			checkPodStatus(t, p, cocoa.StatusStopped)

			require.NoError(t, p.Stop(ctx))

			checkPodStatus(t, p, cocoa.StatusStopped)
		},
		"DeleteSucceeds": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeSecretEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			defer cleanupPod(ctx, t, p, c, v)

			checkPodStatus(t, p, cocoa.StatusStarting)

			require.NoError(t, p.Delete(ctx))

			checkPodStatus(t, p, cocoa.StatusDeleted)
		},
		"DeleteSucceedsWithSecrets": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			secret := cocoa.NewEnvironmentVariable().SetName(t.Name()).
				SetSecretOptions(*cocoa.NewSecretOptions().SetName(testutil.NewSecretName(t.Name())).SetValue("value1"))
			ownedSecret := cocoa.NewEnvironmentVariable().SetName("secret2").
				SetSecretOptions(*cocoa.NewSecretOptions().SetName(testutil.NewSecretName(t.Name())).SetValue("value2").SetOwned(true))

			secretOpts := makePodCreationOpts(t).AddContainerDefinitions(*makeContainerDef(t).AddEnvironmentVariables(*secret, *ownedSecret))

			p, err := pc.CreatePod(ctx, secretOpts)
			require.NoError(t, err)

			defer cleanupPod(ctx, t, p, c, v)

			checkPodStatus(t, p, cocoa.StatusStarting)

			res := p.Resources()
			require.Len(t, res.Containers, 1)
			require.Len(t, res.Containers[0].Secrets, 2)
			val, err := v.GetValue(ctx, utility.FromStringPtr(res.Containers[0].Secrets[0].Name))
			require.NoError(t, err)
			assert.Equal(t, utility.FromStringPtr(secret.SecretOpts.Value), val)

			val, err = v.GetValue(ctx, utility.FromStringPtr(res.Containers[0].Secrets[1].Name))
			require.NoError(t, err)
			assert.Equal(t, utility.FromStringPtr(ownedSecret.SecretOpts.Value), val)

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, c, v, p)
		},
		"DeleteIsIdempotent": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c cocoa.ECSClient, v cocoa.Vault) {
			opts := makePodCreationOpts(t).AddContainerDefinitions(
				*makeContainerDef(t).AddEnvironmentVariables(
					*makeEnvVar(t),
				),
			)
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, c, v, p)

			require.NoError(t, p.Delete(ctx))

			checkPodDeleted(ctx, t, c, v, p)
		},
	}
}

// cleanupPod cleans up all resources regardless of whether they're owned by the
// pod or not.
func cleanupPod(ctx context.Context, t *testing.T, p cocoa.ECSPod, c cocoa.ECSClient, v cocoa.Vault) {
	res := p.Resources()

	_, err := c.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: res.TaskDefinition.ID,
	})
	assert.NoError(t, err)

	_, err = c.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: res.Cluster,
		Task:    res.TaskID,
	})
	assert.NoError(t, err)

	for _, containerRes := range res.Containers {
		for _, s := range containerRes.Secrets {
			assert.NoError(t, v.DeleteSecret(ctx, utility.FromStringPtr(s.Name)))
		}
	}
}

// checkPodDeleted checks that the pod is deleted and all of its owned
// resources are deleted.
func checkPodDeleted(ctx context.Context, t *testing.T, c cocoa.ECSClient, v cocoa.Vault, p cocoa.ECSPod) {
	checkPodStatus(t, p, cocoa.StatusDeleted)

	res := p.Resources()

	describeTasks, err := c.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Tasks:   []*string{res.TaskID},
		Cluster: res.Cluster,
	})
	require.NoError(t, err)
	require.Empty(t, describeTasks.Failures)
	require.Len(t, describeTasks.Tasks, 1)
	assert.Equal(t, ecs.DesiredStatusStopped, utility.FromStringPtr(describeTasks.Tasks[0].DesiredStatus))

	require.NotZero(t, res.TaskDefinition)
	if utility.FromBoolPtr(res.TaskDefinition.Owned) {
		describeTaskDef, err := c.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: res.TaskDefinition.ID,
		})
		require.NoError(t, err)
		require.NotZero(t, describeTaskDef.TaskDefinition)
		assert.NotZero(t, describeTaskDef.TaskDefinition.DeregisteredAt)
	}

	for _, containerRes := range res.Containers {
		for _, secret := range containerRes.Secrets {
			if utility.FromBoolPtr(secret.Owned) {
				_, err := v.GetValue(ctx, utility.FromStringPtr(secret.Name))
				require.Error(t, err)
			} else {
				val, err := v.GetValue(ctx, utility.FromStringPtr(secret.Name))
				require.NoError(t, err)
				assert.NotZero(t, utility.FromStringPtr(secret.Value), val)
			}
		}
	}
}

// checkPodStatus checks that the current pod status matches the expected one.
func checkPodStatus(t *testing.T, p cocoa.ECSPod, status cocoa.ECSStatus) {
	stat := p.StatusInfo()
	assert.Equal(t, status, stat.Status)
	for _, container := range stat.Containers {
		assert.Equal(t, status, container.Status)
	}
}
