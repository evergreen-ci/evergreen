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

func TestNewTerminatePodJob(t *testing.T) {
	podID := "id"
	reason := "reason"
	j, ok := NewTerminatePodJob(podID, reason, utility.RoundPartOfMinute(0)).(*terminatePodJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
	assert.Equal(t, reason, j.Reason)
	assert.Equal(t, podID, j.PodID)
}

func TestTerminatePodJob(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *terminatePodJob){
		"TerminatesAndDeletesResourcesForRunningPod": func(ctx context.Context, t *testing.T, j *terminatePodJob) {
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

			info, err := j.ecsPod.Info(ctx)
			require.NoError(t, err)
			assert.Equal(t, cocoa.StatusDeleted, info.Status)
		},
		"SucceedsWithPodFromDB": func(ctx context.Context, t *testing.T, j *terminatePodJob) {
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

			info, err := j.ecsPod.Info(ctx)
			require.NoError(t, err)
			assert.Equal(t, cocoa.StatusDeleted, info.Status)
		},
		"FailsWhenDeletingResourcesErrors": func(ctx context.Context, t *testing.T, j *terminatePodJob) {
			require.NoError(t, j.pod.Insert())
			j.pod.Resources.ID = utility.RandomString()
			j.ecsPod = nil

			j.Run(ctx)
			require.Error(t, j.Error())

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotEqual(t, pod.StatusTerminated, dbPod.Status)
		},
		"FailsWhenPodStatusUpdateErrors": func(ctx context.Context, t *testing.T, j *terminatePodJob) {
			require.NoError(t, j.pod.Insert())
			j.pod.Status = pod.StatusInitializing

			j.Run(ctx)
			require.Error(t, j.Error())

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotEqual(t, pod.StatusTerminated, j.pod.Status)
		},
		"TerminatesWithoutDeletingResourcesForInitializingPod": func(ctx context.Context, t *testing.T, j *terminatePodJob) {
			j.pod.Status = pod.StatusInitializing
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			dbPod, err := pod.FindOneByID(j.pod.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
		"NoopsforTerminatedPod": func(ctx context.Context, t *testing.T, j *terminatePodJob) {
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
					EnvSecrets: map[string]string{
						"secret0": utility.RandomString(),
						"secret1": utility.RandomString(),
					},
				},
			}

			j, ok := NewTerminatePodJob(p.ID, "reason", utility.RoundPartOfMinute(0)).(*terminatePodJob)
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
			for name, val := range p.TaskContainerCreationOpts.EnvSecrets {
				envVars = append(envVars, *cocoa.NewEnvironmentVariable().
					SetName(name).
					SetSecretOptions(*cocoa.NewSecretOptions().
						SetName(name).
						SetValue(val).
						SetOwned(true)))
			}

			containerDef := cocoa.NewECSContainerDefinition().
				SetImage(p.TaskContainerCreationOpts.Image).
				AddEnvironmentVariables(envVars...)

			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(cluster).
				SetExecutionRole("execution_role")

			ecsPod, err := pc.CreatePod(ctx, cocoa.NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(p.TaskContainerCreationOpts.MemoryMB).
				SetCPU(p.TaskContainerCreationOpts.CPU).
				SetTaskRole("task_role").
				SetExecutionOptions(*execOpts))
			require.NoError(t, err)
			j.ecsPod = ecsPod

			info, err := j.ecsPod.Info(ctx)
			require.NoError(t, err)
			j.pod.Resources.ID = utility.FromStringPtr(info.Resources.TaskID)
			j.pod.Resources.DefinitionID = utility.FromStringPtr(info.Resources.TaskDefinition.ID)
			j.pod.Resources.Cluster = utility.FromStringPtr(info.Resources.Cluster)
			for _, secret := range info.Resources.Secrets {
				j.pod.Resources.SecretIDs = append(j.pod.Resources.SecretIDs, utility.FromStringPtr(secret.Name))
			}

			tCase(ctx, t, j)
		})
	}
}
