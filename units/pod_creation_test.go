package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
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

		assert.NoError(t, db.ClearCollections(pod.Collection, definition.Collection, dispatcher.Collection, event.EventCollection))
	}()

	const clusterName = "cluster"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podCreationJob){
		"Succeeds": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			assert.Equal(t, pod.StatusInitializing, j.pod.Status)

			podDefOpts, err := cloud.ExportECSPodDefinitionOptions(j.env.Settings(), j.pod.TaskContainerCreationOpts)
			require.NoError(t, err)

			pdm, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, nil)
			require.NoError(t, err)
			_, err = pdm.CreatePodDefinition(ctx, *podDefOpts)
			require.NoError(t, err)

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
			assert.Len(t, dbPod.Resources.Containers, 1, "should have created a pod with 1 container")
		},
		"RetriesWithPodDefinitionNotYetCreated": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			assert.Error(t, j.Error())

			assert.True(t, j.RetryInfo().ShouldRetry(), "job should retry because the pod's definition does not yet exist")
		},
		"FailsWithStartingStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusStarting, ""))
			assert.Equal(t, pod.StatusStarting, j.pod.Status)

			j.Run(ctx)
			require.Error(t, j.Error())
			require.Zero(t, j.ecsPod)
			require.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusStarting, dbPod.Status)
		},
		"FailsWithRunningStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusRunning, ""))
			assert.Equal(t, pod.StatusRunning, j.pod.Status)

			j.Run(ctx)
			require.Error(t, j.Error())
			assert.Zero(t, j.ecsPod)
			assert.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusRunning, dbPod.Status)
		},
		"FailsWithTerminatedStatus": func(ctx context.Context, t *testing.T, j *podCreationJob) {
			require.NoError(t, j.pod.Insert())
			require.NoError(t, j.pod.UpdateStatus(pod.StatusTerminated, ""))

			j.Run(ctx)
			require.Error(t, j.Error())
			assert.Zero(t, j.ecsPod)
			assert.Zero(t, j.pod.Resources)
			assert.Len(t, cocoaMock.GlobalECSService.Clusters[clusterName], 0)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(pod.Collection, definition.Collection, dispatcher.Collection, event.EventCollection))

			cocoaMock.ResetGlobalECSService()
			cocoaMock.ResetGlobalSecretCache()
			cocoaMock.GlobalECSService.Clusters[clusterName] = cocoaMock.ECSCluster{}

			env := &mock.Environment{}
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

			p, err := pod.NewTaskIntentPod(evergreen.ECSConfig{AllowedImages: []string{"image"}}, pod.TaskIntentPodOptions{
				MemoryMB:            256,
				CPU:                 512,
				OS:                  pod.OSLinux,
				Arch:                pod.ArchAMD64,
				Image:               "image",
				WorkingDir:          "/working_dir",
				PodSecretExternalID: "pod_secret_external_id",
				PodSecretValue:      "pod_secret_value",
			})
			require.NoError(t, err)

			pd := dispatcher.NewPodDispatcher("group_id", []string{}, []string{p.ID})
			require.NoError(t, pd.Insert())

			j, ok := NewPodCreationJob(p.ID, utility.RoundPartOfMinute(0).Format(TSFormat)).(*podCreationJob)
			require.True(t, ok)

			j.pod = p
			j.env = env

			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(ctx))
			}()
			pc, err := cloud.MakeECSPodCreator(j.ecsClient, nil)
			require.NoError(t, err)
			j.ecsPodCreator = cocoaMock.NewECSPodCreator(pc)

			tCase(ctx, t, j)
		})
	}
}
