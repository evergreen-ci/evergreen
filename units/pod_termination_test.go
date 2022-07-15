package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
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
	defer func() {
		assert.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, dispatcher.Collection, event.LegacyEventLogCollection))
	}()

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
		"TerminatesPodAndResetsTaskStrandedOnPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			b := build.Build{
				Id:     "build_id",
				Status: evergreen.BuildStarted,
			}
			require.NoError(t, b.Insert())
			v := model.Version{
				Id:     "version_id",
				Status: evergreen.VersionStarted,
			}
			require.NoError(t, v.Insert())
			tsk := task.Task{
				Id:                 "task_id",
				Execution:          1,
				BuildId:            b.Id,
				Version:            v.Id,
				ExecutionPlatform:  task.ExecutionPlatformContainer,
				Status:             evergreen.TaskStarted,
				Activated:          true,
				ActivatedTime:      time.Now(),
				DispatchTime:       time.Now(),
				StartTime:          time.Now(),
				LastHeartbeat:      time.Now(),
				ContainerAllocated: true,
			}
			require.NoError(t, tsk.Insert())
			j.pod.RunningTask = tsk.Id
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
			dbPod, err := pod.FindOneByID(j.pod.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)

			dbArchivedTask, err := task.FindOneOldByIdAndExecution(tsk.Id, 1)
			require.NoError(t, err)
			assert.Equal(t, evergreen.TaskFailed, dbArchivedTask.Status, "stranded task should have failed")

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.True(t, dbTask.ShouldAllocateContainer(), "stranded task should have been restarted to re-attempt allocation")

			dbBuild, err := build.FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.Equal(t, evergreen.BuildCreated, dbBuild.Status, "build should have been updated after resetting stranded task")

			dbVersion, err := model.VersionFindOneId(v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionCreated, dbVersion.Status, "version should have been updated after resetting stranded task")
		},
		"RemovesPodFromDispatcher": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			pd := dispatcher.NewPodDispatcher("group_id", []string{"task_id"}, []string{j.pod.ID, "another_pod_id"})
			require.NoError(t, pd.Insert())
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
			dbPod, err := pod.FindOneByID(j.pod.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)

			dbDisp, err := dispatcher.FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Equal(t, dbDisp.PodIDs, []string{"another_pod_id"}, "pod should have been removed from dispatcher's set of pods")
			assert.Equal(t, dbDisp.TaskIDs, []string{"task_id"}, "tasks being dispatched should be unaffected")
		},
		"FixesDispatcherTasksWhenPodIsOnlyRemainingOne": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			t0 := task.Task{
				Id:                     "task_id0",
				Activated:              true,
				Status:                 evergreen.TaskUndispatched,
				ExecutionPlatform:      task.ExecutionPlatformContainer,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
			}
			require.NoError(t, t0.Insert())
			v := model.Version{
				Id:     "version_id",
				Status: evergreen.VersionCreated,
			}
			require.NoError(t, v.Insert())
			b := build.Build{
				Id:      "build_id",
				Version: v.Id,
				Status:  evergreen.BuildCreated,
			}
			require.NoError(t, b.Insert())
			t1 := task.Task{
				Id:                          "task_id1",
				BuildId:                     b.Id,
				Version:                     v.Id,
				Activated:                   true,
				ExecutionPlatform:           task.ExecutionPlatformContainer,
				Status:                      evergreen.TaskUndispatched,
				ContainerAllocated:          true,
				ContainerAllocatedTime:      time.Now(),
				ContainerAllocationAttempts: 100,
			}
			require.NoError(t, t1.Insert())
			pd := dispatcher.NewPodDispatcher("group_id", []string{t0.Id, t1.Id}, []string{j.pod.ID})
			require.NoError(t, pd.Insert())
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
			dbPod, err := pod.FindOneByID(j.pod.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)

			dbDisp, err := dispatcher.FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Empty(t, dbDisp.PodIDs, "terminated pod should have been removed from dispatcher")
			assert.Empty(t, dbDisp.TaskIDs, "tasks should have been cleared from the dispatcher since there are no remaining pods to run them")

			dbTask0, err := task.FindOneId(t0.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask0)
			assert.True(t, dbTask0.ShouldAllocateContainer(), "task should be able to allocate a new container after being removed from the dispatcher")

			dbTask1, err := task.FindOneId(t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			assert.False(t, dbTask1.ShouldAllocateContainer(), "task that has used up all container allocation attempts should not be able to allocate a new container")
			assert.True(t, dbTask1.IsFinished(), "task that has used up all container allocation attempts should be finished")

			dbBuild, err := build.FindOneId(b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.True(t, dbBuild.IsFinished(), "build should be updated after its task has run out of container allocation attempts")

			dbVersion, err := model.VersionFindOneId(v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version should be updated after its task has run out of container allocation attempts")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, dispatcher.Collection, event.LegacyEventLogCollection))
			require.NoError(t, db.CreateCollections(task.Collection, task.OldCollection))

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
