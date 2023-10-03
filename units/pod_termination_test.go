package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPodTerminationJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.TestSpan(ctx, t)

	podID := "id"
	reason := "reason"
	j, ok := NewPodTerminationJob(podID, reason, utility.RoundPartOfMinute(0)).(*podTerminationJob)
	require.True(t, ok)

	assert.NotZero(t, j.ID())
	assert.Equal(t, reason, j.Reason)
	assert.Equal(t, podID, j.PodID)
}

func TestPodTerminationJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	defer func() {
		assert.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, dispatcher.Collection, event.EventCollection))
	}()

	checkCloudPodDeleteRequests := func(t *testing.T, ecsc cocoa.ECSClient) {
		ecsClient, ok := ecsc.(*cocoaMock.ECSClient)
		require.True(t, ok)
		assert.NotZero(t, ecsClient.StopTaskInput, "should have requested to stop the cloud pod")
		assert.Zero(t, ecsClient.DeregisterTaskDefinitionInput, "should not have requested to clean up the ECS task definition")
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podTerminationJob){
		"TerminatesAndDeletesResourcesForRunningPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkCloudPodDeleteRequests(t, j.ecsClient)

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
		},
		"SucceedsWithPodFromDB": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())
			j.pod = nil
			j.ecsPod = nil

			j.Run(ctx)
			require.NoError(t, j.Error())

			checkCloudPodDeleteRequests(t, j.ecsClient)

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
		},
		"SucceedsWhenDeletingNonexistentCloudPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())
			// This simulates a condition where the cloud pod may have been long
			// since cleaned up, so it has no more information about the pod. In
			// the case that the cloud pod does not exist, termination should
			// still succeed.
			j.pod.Resources.ExternalID = utility.RandomString()

			j.Run(ctx)
			assert.NoError(t, j.Error())

			checkCloudPodDeleteRequests(t, j.ecsClient)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
		},
		"SucceedsWhenTerminatingAlreadyTerminatedPod": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			assert.NoError(t, j.Error())
			j.Run(ctx)
			assert.NoError(t, j.Error())

			checkCloudPodDeleteRequests(t, j.ecsClient)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)

			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
		},
		"SucceedsWhenPodTaskAlreadyDeallocated": func(ctx context.Context, t *testing.T, j *podTerminationJob) {
			tsk := task.Task{
				Id:                 "task_id",
				Execution:          1,
				PodID:              j.PodID,
				ExecutionPlatform:  task.ExecutionPlatformContainer,
				Status:             evergreen.TaskFailed,
				Activated:          false,
				ActivatedTime:      time.Now(),
				DispatchTime:       time.Now(),
				StartTime:          time.Now(),
				LastHeartbeat:      time.Now(),
				ContainerAllocated: false,
			}
			require.NoError(t, tsk.Insert())
			j.pod.TaskRuntimeInfo.RunningTaskID = tsk.Id
			j.pod.TaskRuntimeInfo.RunningTaskExecution = tsk.Execution
			require.NoError(t, j.pod.Insert())

			j.Run(ctx)
			assert.NoError(t, j.Error())

			checkCloudPodDeleteRequests(t, j.ecsClient)

			dbPod, err := pod.FindOneByID(j.PodID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusTerminated, dbPod.Status)
			assert.Equal(t, cocoa.StatusDeleted, j.ecsPod.StatusInfo().Status)
			assert.Empty(t, dbPod.TaskRuntimeInfo)

			dbTask, err := task.FindOneId(tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask)
			assert.Equal(t, evergreen.TaskFailed, dbTask.Status)
			assert.False(t, dbTask.ContainerAllocated)
			assert.Equal(t, 1, dbTask.Execution)
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
				PodID:              j.PodID,
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
			j.pod.TaskRuntimeInfo.RunningTaskID = tsk.Id
			j.pod.TaskRuntimeInfo.RunningTaskExecution = tsk.Execution
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
				PodID:                  j.pod.ID,
				Project:                "project-ref",
			}
			require.NoError(t, t0.Insert())
			projectRef := model.ProjectRef{
				Identifier: "project-ref",
			}
			require.NoError(t, projectRef.Insert())
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
			pp := model.ParserProject{
				Id: v.Id,
			}
			require.NoError(t, pp.Insert())
			t1 := task.Task{
				Id:                          "task_id1",
				BuildId:                     b.Id,
				Version:                     v.Id,
				Project:                     projectRef.Identifier,
				PodID:                       j.PodID,
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
			tctx, cancel := context.WithCancel(ctx)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

			require.NoError(t, db.ClearCollections(pod.Collection, task.Collection, task.OldCollection, build.Collection, model.VersionCollection, dispatcher.Collection, event.EventCollection))

			cluster := "cluster"

			cocoaMock.ResetGlobalECSService()
			cocoaMock.GlobalECSService.Clusters[cluster] = cocoaMock.ECSCluster{}
			defer func() {
				cocoaMock.ResetGlobalECSService()
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
				},
			}

			j, ok := NewPodTerminationJob(p.ID, "reason", utility.RoundPartOfMinute(0)).(*podTerminationJob)
			require.True(t, ok)
			j.pod = &p
			j.PodID = p.ID
			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			j.env = env
			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(tctx))
			}()
			j.ecsPod = generateTestingECSPod(tctx, t, j.ecsClient, cluster, p.TaskContainerCreationOpts)
			j.pod.Resources = cloud.ImportECSPodResources(j.ecsPod.Resources())

			tCase(tctx, t, j)
		})
	}
}

// generateTestingECSPod creates a pod in ECS from the given options. The
// cluster must exist before this is called.
func generateTestingECSPod(ctx context.Context, t *testing.T, client cocoa.ECSClient, cluster string, creationOpts pod.TaskContainerCreationOptions) cocoa.ECSPod {
	pc, err := ecs.NewBasicPodCreator(*ecs.NewBasicPodCreatorOptions().SetClient(client))
	require.NoError(t, err)

	containerDef := cocoa.NewECSContainerDefinition().
		SetImage(creationOpts.Image).
		SetCommand([]string{"echo", "hello"})

	execOpts := cocoa.NewECSPodExecutionOptions().SetCluster(cluster)
	defOpts := cocoa.NewECSPodDefinitionOptions().
		AddContainerDefinitions(*containerDef).
		SetMemoryMB(creationOpts.MemoryMB).
		SetCPU(creationOpts.CPU).
		SetTaskRole("task_role").
		SetExecutionRole("execution_role")

	pdm, err := cloud.MakeECSPodDefinitionManager(client, nil)
	require.NoError(t, err)
	item, err := pdm.CreatePodDefinition(ctx, *defOpts)
	require.NoError(t, err)

	podDef, err := definition.FindOneByExternalID(item.ID)
	require.NoError(t, err)
	require.NotZero(t, podDef, "pod definition should have been cached")

	ecsPod, err := pc.CreatePodFromExistingDefinition(ctx, cloud.ExportECSPodDefinition(*podDef), *execOpts)
	require.NoError(t, err)
	return ecsPod
}
