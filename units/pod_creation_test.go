package units

import (
	"context"
	"strings"
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
	"github.com/mongodb/amboy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPodCreationJob(t *testing.T) {
	podID := utility.RandomString()

	j, ok := NewPodCreationJob(podID, utility.RoundPartOfMinute(0).Format(TSFormat)).(*podCreationJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
	assert.Equal(t, podID, j.PodID)
}

func TestPodCreationJob(t *testing.T) {
	const clusterName = "cluster"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podCreationJob){
		"Succeeds": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			assert.Equal(t, pod.StatusInitializing, j.pod.Status)

			j.Run(ctx)
			require.NoError(t, j.Error())

			res := j.ecsPod.Resources()
			assert.Equal(t, cocoa.StatusStarting, j.ecsPod.StatusInfo().Status)
			assert.Equal(t, pod.StatusStarting, j.pod.Status)
			assert.Equal(t, clusterName, utility.FromStringPtr(res.Cluster))
			assert.Equal(t, j.pod.Resources.DefinitionID, utility.FromStringPtr(res.TaskDefinition.ID))
			assert.Equal(t, j.pod.Resources.ExternalID, utility.FromStringPtr(res.TaskID))
			require.Len(t, res.Containers, 1)
			require.Len(t, res.Containers[0].Secrets, 2)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 1)
			assert.Len(t, cocoaMock.GlobalSecretCache, 2)
			for _, secret := range res.Containers[0].Secrets {
				id := utility.FromStringPtr(secret.ID)
				assert.Contains(t, j.pod.Resources.Containers[0].SecretIDs, id)

				val, err := j.vault.GetValue(ctx, id)
				require.NoError(t, err)

				var found bool
				for k, v := range j.pod.TaskContainerCreationOpts.EnvSecrets {
					if strings.Contains(utility.FromStringPtr(secret.Name), k) {
						assert.Equal(t, v.Value, val)
						found = true
						break
					}
				}
				assert.True(t, found, "could not find secret '%s'", secret.Name)
			}

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusStarting, dbPod.Status)
		},
		"FailsWithStartingStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusStarting))
			assert.Equal(t, pod.StatusStarting, j.pod.Status)

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusStarting, dbPod.Status)
		},
		"FailsWithRunningStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusRunning))
			assert.Equal(t, pod.StatusRunning, j.pod.Status)

			j.Run(ctx)
			require.Error(t, j.Error())
			assert.Zero(t, j.ecsPod)
			assert.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusRunning, dbPod.Status)
		},
		"FailsWithTerminatedStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusTerminated))

			j.Run(ctx)
			require.Error(t, j.Error())
			assert.Zero(t, j.ecsPod)
			assert.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
		"DecommissionsInitializingPodWithBadContainerOptions": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			j.pod.TaskContainerCreationOpts = pod.TaskContainerCreationOptions{}
			require.NoError(t, j.pod.Insert())

			j.UpdateRetryInfo(amboy.JobRetryOptions{
				MaxAttempts: utility.ToIntPtr(1),
			})

			j.Run(ctx)
			require.Error(t, j.pod.Insert())
			assert.Zero(t, j.ecsPod)
			assert.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalSecretCache, 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusDecommissioned, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			}()

			cocoaMock.GlobalECSService = cocoaMock.ECSService{
				Clusters: map[string]cocoaMock.ECSCluster{
					clusterName: {},
				},
				TaskDefs: map[string][]cocoaMock.ECSTaskDefinition{},
			}
			cocoaMock.GlobalSecretCache = map[string]cocoaMock.StoredSecret{}

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
					EnvSecrets: map[string]pod.Secret{
						"SECRET_ENV_VAR0": {
							Value:  utility.RandomString(),
							Exists: utility.FalsePtr(),
							Owned:  utility.TruePtr(),
						},
						"SECRET_ENV_VAR1": {
							Value:  utility.RandomString(),
							Exists: utility.FalsePtr(),
							Owned:  utility.TruePtr(),
						},
					},
				},
			}

			env := &evgMock.Environment{}
			require.NoError(t, env.Configure(ctx))
			var envClusters []evergreen.ECSClusterConfig
			for name := range cocoaMock.GlobalECSService.Clusters {
				envClusters = append(envClusters, evergreen.ECSClusterConfig{
					Name:     name,
					Platform: evergreen.ECSClusterPlatformLinux,
				})
			}
			env.EvergreenSettings.Providers.AWS.Pod.ECS.Clusters = envClusters

			j, ok := NewPodCreationJob(p.ID, utility.RoundPartOfMinute(0).Format(TSFormat)).(*podCreationJob)
			require.True(t, ok)

			j.pod = &p
			j.env = env

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
