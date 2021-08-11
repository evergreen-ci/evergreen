package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	evgMock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCreatePodJob(t *testing.T) {
	podID := utility.RandomString()
	p := pod.Pod{
		ID:     podID,
		Status: pod.StatusInitializing,
	}

	j, ok := NewCreatePodJob(&evgMock.Environment{}, &p, utility.RoundPartOfMinute(0).Format(TSFormat)).(*createPodJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
	assert.Equal(t, podID, j.PodID)
	assert.Equal(t, pod.StatusInitializing, j.pod.Status)
}

func TestCreatePodJob(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *createPodJob){
		"Succeeds": func(ctx context.Context, t *testing.T, j *createPodJob) {
			require.NoError(t, j.pod.Insert())
			assert.Equal(t, pod.StatusInitializing, j.pod.Status)

			j.Run(ctx)
			require.NoError(t, j.Error())

			res := j.ecsPod.Resources()
			assert.Equal(t, cocoa.StatusStarting, j.ecsPod.StatusInfo().Status)
			assert.Equal(t, pod.StatusStarting, j.pod.Status)
			assert.Equal(t, "cluster", utility.FromStringPtr(res.Cluster))
			assert.Equal(t, j.pod.Resources.DefinitionID, utility.FromStringPtr(res.TaskDefinition.ID))
			assert.Equal(t, j.pod.Resources.ExternalID, utility.FromStringPtr(res.TaskID))
			require.Len(t, res.Containers, 1)
			require.Len(t, res.Containers[0].Secrets, 2)
			assert.Len(t, cocoaMock.GlobalSecretCache, 2)
			for _, secret := range res.Containers[0].Secrets {
				id := utility.FromStringPtr(secret.Name)
				assert.Contains(t, j.pod.Resources.Containers[0].SecretIDs, id)
				val, err := j.vault.GetValue(ctx, id)
				require.NoError(t, err)
				assert.Equal(t, utility.FromStringPtr(secret.Value), val)
			}

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusStarting, dbPod.Status)
		},
		"FailsWithStartingStatus": func(ctx context.Context, t *testing.T, j *createPodJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusStarting))
			assert.Equal(t, pod.StatusStarting, j.pod.Status)

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusStarting, dbPod.Status)
		},
		"FailsWithRunningStatus": func(ctx context.Context, t *testing.T, j *createPodJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusRunning))
			assert.Equal(t, pod.StatusRunning, j.pod.Status)

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusRunning, dbPod.Status)
		},
		"FailsWithTerminatedStatus": func(ctx context.Context, t *testing.T, j *createPodJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusTerminated))

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
		"FailsWhenExportingOpts": func(ctx context.Context, t *testing.T, j *createPodJob) {
			j.pod.TaskContainerCreationOpts.Image = ""
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusInitializing, dbPod.Status)
		},
		"FailsWhenCreatingPod": func(ctx context.Context, t *testing.T, j *createPodJob) {
			j.pod.TaskContainerCreationOpts.CPU = 0
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusInitializing, dbPod.Status)
		},
		"FailsWithMismatchedPlatformOS": func(ctx context.Context, t *testing.T, j *createPodJob) {
			j.pod.TaskContainerCreationOpts.OS = "windows"
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusInitializing, dbPod.Status)
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
			cocoaMock.GlobalECSService.Clusters[cluster] = cocoaMock.ECSCluster{}

			defer func() {
				cocoaMock.GlobalECSService = cocoaMock.ECSService{
					Clusters: map[string]cocoaMock.ECSCluster{},
					TaskDefs: map[string][]cocoaMock.ECSTaskDefinition{},
				}
				cocoaMock.GlobalSecretCache = map[string]cocoaMock.StoredSecret{}
			}()

			p := pod.Pod{
				ID:     "id",
				Status: pod.StatusInitializing,
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					Image:    "image",
					MemoryMB: 128,
					CPU:      128,
					OS:       pod.OSLinux,
					Arch:     pod.ArchAMD64,
					EnvSecrets: map[string]string{
						"s0": utility.RandomString(),
						"s1": utility.RandomString(),
					},
				},
			}

			env := evgMock.Environment{}
			require.NoError(t, env.Configure(ctx))
			var envClusters []evergreen.ECSClusterConfig
			for name := range cocoaMock.GlobalECSService.Clusters {
				envClusters = append(envClusters, evergreen.ECSClusterConfig{
					Name:     name,
					Platform: evergreen.ECSClusterPlatformLinux,
				})
			}
			env.EvergreenSettings.Providers.AWS.Pod.ECS.Clusters = envClusters

			j, ok := NewCreatePodJob(&env, &p, utility.RoundPartOfMinute(0).Format(TSFormat)).(*createPodJob)
			require.True(t, ok)

			j.pod = &p

			j.smClient = &cocoaMock.SecretsManagerClient{}
			defer func() {
				assert.NoError(t, j.smClient.Close(ctx))
			}()

			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(ctx))
			}()

			j.vault = cloud.MakeSecretsManagerVault(j.smClient)

			pc, err := ecs.NewBasicECSPodCreator(j.ecsClient, j.vault)
			require.NoError(t, err)
			j.ecsPodCreator = pc

			tCase(ctx, t, j)
		})
	}
}
