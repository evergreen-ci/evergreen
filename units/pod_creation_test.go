package units

import (
	"context"
	"strings"
	"testing"

	"github.com/evergreen-ci/cocoa"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	evgMock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/utility"
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
	defer func() {
		cocoaMock.ResetGlobalECSService()
		cocoaMock.ResetGlobalSecretCache()
	}()

	const clusterName = "cluster"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podCreationJob){
		"Succeeds": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			assert.Equal(t, pod.StatusInitializing, j.pod.Status)

			j.Run(ctx)
			require.NoError(t, j.Error())

			res := j.ecsPod.Resources()
			assert.Equal(t, cocoa.StatusStarting, j.ecsPod.StatusInfo().Status)
			assert.Equal(t, clusterName, utility.FromStringPtr(res.Cluster))
			require.Len(t, res.Containers, 1)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)

			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 1, "should have started an ECS task in the cluster")

			assert.Equal(t, pod.StatusStarting, dbPod.Status)
			taskDefID := utility.FromStringPtr(res.TaskDefinition.ID)
			assert.NotZero(t, taskDefID)
			assert.Equal(t, dbPod.Resources.DefinitionID, taskDefID, "ECS task definition ID should be saved in the DB pod")
			taskID := utility.FromStringPtr(res.TaskID)
			assert.NotZero(t, taskID)
			assert.Equal(t, dbPod.Resources.ExternalID, taskID, "ECS task ID should be saved in the DB pod")
			require.Len(t, dbPod.Resources.Containers, 1, "should have created a pod with 1 container")

			assert.Len(t, cocoaMock.GlobalSecretCache, len(dbPod.TaskContainerCreationOpts.EnvSecrets))
			containerResources := res.Containers[0]
			require.Len(t, containerResources.Secrets, len(dbPod.TaskContainerCreationOpts.EnvSecrets))
			for _, secret := range containerResources.Secrets {
				id := utility.FromStringPtr(secret.ID)
				assert.Contains(t, dbPod.Resources.Containers[0].SecretIDs, id)

				val, err := j.vault.GetValue(ctx, id)
				require.NoError(t, err)

				name := utility.FromStringPtr(secret.Name)

				var found bool
				for k, v := range dbPod.TaskContainerCreationOpts.EnvSecrets {
					if strings.Contains(name, k) {
						assert.Equal(t, v.Value, val)
						found = true
						break
					}
				}
				assert.True(t, found, "missing expected secret '%s'", name)
			}
		},
		// "FailsWithStartingStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
		//     require.NoError(t, j.pod.Insert())
		//     require.NoError(t, j.pod.UpdateStatus(pod.StatusStarting))
		//     assert.Equal(t, pod.StatusStarting, j.pod.Status)
		//
		//     j.Run(ctx)
		//     require.Error(t, j.Error())
		//     require.Zero(t, j.ecsPod)
		//     require.Zero(t, j.pod.Resources)
		//     assert.Len(t, cocoaMock.GlobalSecretCache, 0)
		//     assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)
		//
		//     dbPod, err := pod.FindOneByID(j.PodID)
		//     require.NoError(t, err)
		//     require.NotZero(t, dbPod)
		//     assert.Equal(t, pod.StatusStarting, dbPod.Status)
		// },
		// "FailsWithRunningStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
		//     require.NoError(t, j.pod.Insert())
		//     require.NoError(t, j.pod.UpdateStatus(pod.StatusRunning))
		//     assert.Equal(t, pod.StatusRunning, j.pod.Status)
		//
		//     j.Run(ctx)
		//     require.Error(t, j.Error())
		//     assert.Zero(t, j.ecsPod)
		//     assert.Zero(t, j.pod.Resources)
		//     assert.Len(t, cocoaMock.GlobalSecretCache, 0)
		//     assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)
		//
		//     dbPod, err := pod.FindOneByID(j.PodID)
		//     require.NoError(t, err)
		//     require.NotZero(t, dbPod)
		//     assert.Equal(t, pod.StatusRunning, dbPod.Status)
		// },
		// "FailsWithTerminatedStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
		//     require.NoError(t, j.pod.Insert())
		//     require.NoError(t, j.pod.UpdateStatus(pod.StatusTerminated))
		//
		//     j.Run(ctx)
		//     require.Error(t, j.Error())
		//     assert.Zero(t, j.ecsPod)
		//     assert.Zero(t, j.pod.Resources)
		//     assert.Len(t, cocoaMock.GlobalSecretCache, 0)
		//     assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)
		//
		//     dbPod, err := pod.FindOneByID(j.PodID)
		//     require.NoError(t, err)
		//     require.NotZero(t, dbPod)
		//     assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		// },
		// "DecommissionsInitializingPodWithBadContainerOptions": func(ctx context.Context, t *testing.T, j *podCreationJob) {
		//     j.pod.TaskContainerCreationOpts = pod.TaskContainerCreationOptions{}
		//     require.NoError(t, j.pod.Insert())
		//
		//     j.UpdateRetryInfo(amboy.JobRetryOptions{
		//         MaxAttempts: utility.ToIntPtr(1),
		//     })
		//
		//     j.Run(ctx)
		//     require.Error(t, j.pod.Insert())
		//     assert.Zero(t, j.ecsPod)
		//     assert.Zero(t, j.pod.Resources)
		//     assert.Len(t, cocoaMock.GlobalSecretCache, 0)
		//
		//     dbPod, err := pod.FindOneByID(j.PodID)
		//     require.NoError(t, err)
		//     require.NotZero(t, dbPod)
		//     assert.Equal(t, pod.StatusDecommissioned, dbPod.Status)
		// },
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(pod.Collection, definition.Collection, event.LegacyEventLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(pod.Collection, definition.Collection, event.LegacyEventLogCollection))
			}()

			cocoaMock.ResetGlobalECSService()
			cocoaMock.GlobalECSService.Clusters[clusterName] = cocoaMock.ECSCluster{}
			cocoaMock.ResetGlobalSecretCache()

			env := &evgMock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Providers.AWS.Pod.ECS.Clusters = []evergreen.ECSClusterConfig{
				{
					Name: clusterName,
					OS:   evergreen.ECSOSLinux,
				},
			}
			env.EvergreenSettings.Providers.AWS.Pod.ECS.CapacityProviders = []evergreen.ECSCapacityProvider{
				{
					Name: "capacity_provider",
					OS:   evergreen.ECSOSLinux,
					Arch: evergreen.ECSArchAMD64,
				},
			}

			p, err := pod.NewTaskIntentPod(pod.TaskIntentPodOptions{
				MemoryMB:   256,
				CPU:        512,
				OS:         pod.OSLinux,
				Arch:       pod.ArchAMD64,
				Image:      "image",
				WorkingDir: "/working_dir",
			})
			require.NoError(t, err)

			smClient := &cocoaMock.SecretsManagerClient{}
			defer func() {
				assert.NoError(t, smClient.Close(ctx))
			}()
			vault := cocoaMock.NewVault(cloud.MakeSecretsManagerVault(smClient))

			ecsClient := &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, ecsClient.Close(ctx))
			}()

			podDefOpts, err := cloud.ExportECSPodDefinitionOptions(env.Settings(), p.TaskContainerCreationOpts)
			require.NoError(t, err)

			pdm, err := cloud.MakeECSPodDefinitionManager(ecsClient, vault)
			require.NoError(t, err)
			_, err = pdm.CreatePodDefinition(ctx, *podDefOpts)
			require.NoError(t, err)

			j, ok := NewPodCreationJob(p.ID, utility.RoundPartOfMinute(0).Format(TSFormat)).(*podCreationJob)
			require.True(t, ok)

			j.pod = p
			j.env = env

			j.ecsClient = ecsClient
			j.smClient = smClient
			j.vault = vault

			pc, err := cloud.MakeECSPodCreator(j.ecsClient, j.vault)
			require.NoError(t, err)
			j.ecsPodCreator = cocoaMock.NewECSPodCreator(pc)

			tCase(ctx, t, j)
		})
	}
}
