package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPodTerminationJob(t *testing.T) {
	podID := "id"
	reason := "reason"
	j, ok := NewPodTerminationJob(podID, reason, utility.RoundPartOfMinute(0)).(*podTerminationJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
	assert.Equal(t, reason, j.Reason)
	assert.Equal(t, podID, j.PodID)
}

func TestPodTerminationJob(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podTerminationJob){
		"TerminatesAndDeletesResourcesForRunningPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			ecsClient, ok := j.ecsClient.(*mock.ECSClient)
			require.True(t, ok)
			assert.NotZero(t, ecsClient.StopTaskInput)
			assert.NotZero(t, ecsClient.DeregisterTaskDefinitionInput)

			smClient, ok := j.smClient.(*mock.SecretsManagerClient)
			require.True(t, ok)
			assert.NotZero(t, smClient.DeleteSecretInput)

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
		},
		"SucceedsWithPodFromDB": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())
			j.pod = nil
			j.ecsPod = nil

			j.Run(ctx)
			require.NoError(t, j.Error())

			ecsClient, ok := j.ecsClient.(*mock.ECSClient)
			require.True(t, ok)
			assert.NotZero(t, ecsClient.StopTaskInput)
			assert.NotZero(t, ecsClient.DeregisterTaskDefinitionInput)

			smClient, ok := j.smClient.(*mock.SecretsManagerClient)
			require.True(t, ok)
			assert.NotZero(t, smClient.DeleteSecretInput)

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
		},
		"FailsWhenDeletingResourcesErrors": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())
			j.pod.Resources.ExternalID = utility.RandomString()
			j.ecsPod = nil

			j.Run(ctx)
			require.Error(t, j.Error())

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotEqual(t, pod.StatusTerminated, dbPod.Status)
		},
		"TerminatesWithoutDeletingResourcesForIntentPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			j.pod.Status = pod.StatusInitializing
			j.pod.Resources = pod.ResourceInfo{}
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			dbPod, err := pod.FindOneByID(j.pod.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
		"NoopsforTerminatedPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			j.pod.Status = pod.StatusTerminated
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			dbPod, err := pod.FindOneByID(j.pod.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			}()

			cluster := "cluster"

			mock.GlobalECSService.Clusters[cluster] = mock.ECSCluster{}
			defer func() {
				mock.GlobalECSService = mock.ECSService{
					Clusters: map[string]mock.ECSCluster{},
					TaskDefs: map[string][]mock.ECSTaskDefinition{},
				}
			}()

			p := pod.Pod{
				ID:     "id",
				Status: pod.StatusRunning,
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					Image:    "image",
					MemoryMB: 128,
					CPU:      128,
					OS:       pod.OSLinux,
					Arch:     pod.ArchAMD64,
					EnvSecrets: map[string]pod.Secret{
						"SECRET_ENV_VAR0": {
							Value: utility.RandomString(),
						},
						"SECRET_ENV_VAR1": {
							Value: utility.RandomString(),
						},
					},
				},
			}

			j, ok := NewPodTerminationJob(p.ID, "reason", utility.RoundPartOfMinute(0)).(*podTerminationJob)
			require.True(t, ok)
			j.pod = &p
			j.smClient = &mock.SecretsManagerClient{}
			defer func() {
				assert.NoError(t, j.smClient.Close(ctx))
			}()
			j.ecsClient = &mock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(ctx))
			}()
			j.vault = cloud.MakeSecretsManagerVault(j.smClient)

			pc, err := ecs.NewBasicECSPodCreator(j.ecsClient, j.vault)
			require.NoError(t, err)

			var envVars []cocoa.EnvironmentVariable
			for name, secret := range p.TaskContainerCreationOpts.EnvSecrets {
				envVars = append(envVars, *cocoa.NewEnvironmentVariable().
					SetName(name).
					SetSecretOptions(*cocoa.NewSecretOptions().
						SetName(name).
						SetNewValue(secret.Value).
						SetOwned(true)))
			}

			containerDef := cocoa.NewECSContainerDefinition().
				SetImage(p.TaskContainerCreationOpts.Image).
				AddEnvironmentVariables(envVars...)

			execOpts := cocoa.NewECSPodExecutionOptions().SetCluster(cluster)

			ecsPod, err := pc.CreatePod(ctx, *cocoa.NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(p.TaskContainerCreationOpts.MemoryMB).
				SetCPU(p.TaskContainerCreationOpts.CPU).
				SetTaskRole("task_role").
				SetExecutionRole("execution_role").
				SetExecutionOptions(*execOpts))
			require.NoError(t, err)
			j.ecsPod = ecsPod

			res := j.ecsPod.Resources()
			j.pod.Resources = cloud.ImportECSPodResources(res)

			tCase(ctx, t, j)
		})
	}
}
