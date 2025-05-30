package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUpsertAtomically(t *testing.T) {
	defer func() {
		assert.NoError(t, db.DropCollections(Collection))
	}()
	require.NoError(t, testutil.AddTestIndexes(Collection, true, false, GroupIDKey))

	for tName, tCase := range map[string]func(t *testing.T, pd PodDispatcher){
		"InsertsNewPodDispatcher": func(t *testing.T, pd PodDispatcher) {
			change, err := pd.UpsertAtomically(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, change.Updated)

			dbDispatcher, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
		},
		"UpdatesExistingPodDispatcher": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert(t.Context()))

			change, err := pd.UpsertAtomically(t.Context())
			require.NoError(t, err)
			require.Equal(t, 1, change.Updated)

			dbDispatcher, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
			assert.NotZero(t, pd.ModificationCount)
			assert.Equal(t, pd.ModificationCount, dbDispatcher.ModificationCount)
			assert.False(t, utility.IsZeroTime(dbDispatcher.LastModified))
			assert.Equal(t, pd.LastModified, dbDispatcher.LastModified)
		},
		"FailsWithMatchingGroupIDButDifferentDispatcherID": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert(t.Context()))

			modified := pd
			modified.ID = primitive.NewObjectID().Hex()
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically(t.Context())
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(t.Context(), modified.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbDispatcher)

			dbDispatcher, err = FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
		"FailsWithMatchingDispatcherIDButDifferentGroupID": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert(t.Context()))

			modified := pd
			modified.GroupID = utility.RandomString()
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically(t.Context())
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
		"FailsWithDifferentModificationCount": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert(t.Context()))

			modified := pd
			modified.ModificationCount = 12345
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically(t.Context())
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t, NewPodDispatcher("group0", []string{"task0"}, []string{"pod0"}))
		})
	}
}

func TestAssignNextTask(t *testing.T) {
	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	evergreen.SetEnvironment(env)
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, event.EventCollection))
	}()

	getDispatchableTask := func() task.Task {
		return task.Task{
			Id:                 utility.RandomString(),
			Activated:          true,
			ActivatedTime:      time.Now(),
			Status:             evergreen.TaskUndispatched,
			ContainerAllocated: true,
			ExecutionPlatform:  task.ExecutionPlatformContainer,
			DisplayTaskId:      utility.ToStringPtr(""),
		}
	}
	getProjectRef := func() model.ProjectRef {
		return model.ProjectRef{
			Id:         utility.RandomString(),
			Identifier: utility.RandomString(),
			Enabled:    true,
		}
	}

	type testCaseParams struct {
		env        evergreen.Environment
		dispatcher PodDispatcher
		pod        pod.Pod
		task       task.Task
		ref        model.ProjectRef
	}

	checkTaskDispatchedToPod := func(t *testing.T, tsk task.Task, p pod.Pod) {
		dbTask, err := task.FindOneId(t.Context(), tsk.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.Equal(t, evergreen.TaskDispatched, dbTask.Status)
		assert.False(t, utility.IsZeroTime(dbTask.DispatchTime))
		assert.False(t, utility.IsZeroTime(dbTask.LastHeartbeat))
		assert.Equal(t, p.ID, dbTask.PodID)
		assert.Equal(t, p.AgentVersion, dbTask.AgentVersion)

		dbPod, err := pod.FindOneByID(t.Context(), p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, tsk.Id, dbPod.TaskRuntimeInfo.RunningTaskID)
		assert.Equal(t, pod.StatusDecommissioned, dbPod.Status)

		taskEvents, err := event.FindAllByResourceID(t.Context(), dbTask.Id)
		require.NoError(t, err)
		require.Len(t, taskEvents, 1)
		assert.Equal(t, event.TaskDispatched, taskEvents[0].EventType)

		podEvents, err := event.FindAllByResourceID(t.Context(), p.ID)
		require.NoError(t, err)
		var foundPodAssignedTask bool
		var foundPodUpdatedStatus bool
		for _, podEvent := range podEvents {
			switch podEvent.EventType {
			case string(event.EventPodAssignedTask):
				foundPodAssignedTask = true
			case string(event.EventPodStatusChange):
				foundPodUpdatedStatus = true
			}
		}
		assert.True(t, foundPodAssignedTask)
		assert.True(t, foundPodUpdatedStatus)
	}

	checkTaskUnallocated := func(t *testing.T, tsk task.Task) {
		dbTask, err := task.FindOneId(t.Context(), tsk.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.False(t, dbTask.ContainerAllocated)
	}

	checkDispatcherTasks := func(t *testing.T, pd PodDispatcher, taskIDs []string) {
		dbDispatcher, err := FindOneByID(t.Context(), pd.ID)
		require.NoError(t, err)
		require.NotZero(t, dbDispatcher)

		require.Len(t, pd.TaskIDs, len(taskIDs))
		for i := range taskIDs {
			assert.Equal(t, taskIDs[i], pd.TaskIDs[i])
		}
	}

	for tName, tCase := range map[string]func(t *testing.T, params testCaseParams){
		"DispatchesTask": func(t *testing.T, params testCaseParams) {
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, params.task.Id, nextTask.Id)

			checkTaskDispatchedToPod(t, params.task, params.pod)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DispatchesExecutionTaskAndUpdatesDisplayTask": func(t *testing.T, params testCaseParams) {
			dt := task.Task{
				Id:             "display-task",
				DisplayOnly:    true,
				ExecutionTasks: []string{params.task.Id},
				Status:         evergreen.TaskUndispatched,
			}
			params.task.DisplayTaskId = utility.ToStringPtr(dt.Id)
			require.NoError(t, dt.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, params.task.Id, nextTask.Id)

			checkTaskDispatchedToPod(t, params.task, params.pod)
			checkDispatcherTasks(t, params.dispatcher, nil)

			dbDisplayTask, err := task.FindOneId(t.Context(), dt.Id)
			require.NoError(t, err)
			require.NotZero(t, dbDisplayTask)

			assert.Equal(t, evergreen.TaskDispatched, dbDisplayTask.Status, "display task should be updated when container execution task dispatches")
		},
		"DispatchesTaskInDisabledHiddenProject": func(t *testing.T, params testCaseParams) {
			params.task.Requester = evergreen.GithubPRRequester
			params.ref.Enabled = false
			params.ref.Hidden = utility.TruePtr()
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, params.task.Id, nextTask.Id)

			checkTaskDispatchedToPod(t, params.task, params.pod)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesNonexistentTaskAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			dbTask, err := task.FindOneId(t.Context(), params.task.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesTaskWithNonexistentProjectAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesDeactivatedTaskAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			params.task.Activated = false
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesUndispatchableTaskAndReturnsNextDispatchableTask": func(t *testing.T, params testCaseParams) {
			params.task.Activated = false
			dispatchableTask0 := getDispatchableTask()
			dispatchableTask0.Project = params.ref.Id
			dispatchableTask1 := getDispatchableTask()
			dispatchableTask1.Project = params.ref.Id
			params.dispatcher.TaskIDs = append(params.dispatcher.TaskIDs, dispatchableTask0.Id, dispatchableTask1.Id)
			require.NoError(t, params.dispatcher.Insert(t.Context()))
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, dispatchableTask0.Insert(t.Context()))
			require.NoError(t, dispatchableTask1.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, dispatchableTask0.Id, nextTask.Id)

			checkTaskUnallocated(t, params.task)
			checkTaskDispatchedToPod(t, dispatchableTask0, params.pod)
			checkDispatcherTasks(t, params.dispatcher, []string{dispatchableTask1.Id})
		},
		"DequeuesTaskWithoutContainerAllocatedAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			require.NoError(t, params.pod.Insert(t.Context()))
			params.task.Status = evergreen.TaskUndispatched
			params.task.ContainerAllocated = false
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesDisabledTaskAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			require.NoError(t, params.pod.Insert(t.Context()))
			params.task.Priority = evergreen.DisabledTaskPriority
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesTaskInDisabledProjectAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			params.ref.Enabled = false
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesTaskInProjectWithDispatchingDisabledAndDoesNotDispatchIt": func(t *testing.T, params testCaseParams) {
			params.ref.DispatchingDisabled = utility.TruePtr()
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"FailsWithPodInStateThatCannotRunTasks": func(t *testing.T, params testCaseParams) {
			params.pod.Status = pod.StatusDecommissioned
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.Error(t, err)
			assert.Zero(t, nextTask)

			checkDispatcherTasks(t, params.dispatcher, []string{params.task.Id})
		},
		"FailsWithPodAlreadyAssignedTask": func(t *testing.T, params testCaseParams) {
			params.pod.TaskRuntimeInfo.RunningTaskID = "running-task"
			require.NoError(t, params.pod.Insert(t.Context()))
			require.NoError(t, params.task.Insert(t.Context()))
			require.NoError(t, params.ref.Insert(t.Context()))
			require.NoError(t, params.dispatcher.Insert(t.Context()))

			nextTask, err := params.dispatcher.AssignNextTask(t.Context(), params.env, &params.pod)
			assert.Error(t, err)
			assert.Zero(t, nextTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {

			require.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, event.EventCollection))

			p := pod.Pod{
				ID:           utility.RandomString(),
				Status:       pod.StatusRunning,
				AgentVersion: evergreen.AgentVersion,
			}
			tsk := getDispatchableTask()
			ref := getProjectRef()
			tsk.Project = ref.Id
			pd := NewPodDispatcher(GetGroupID(&tsk), []string{tsk.Id}, []string{p.ID})

			tCase(t, testCaseParams{
				env:        env,
				dispatcher: pd,
				pod:        p,
				task:       tsk,
				ref:        ref,
			})
		})
	}
}

func TestRemovePod(t *testing.T) {
	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	evergreen.SetEnvironment(env)
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection))
	}()

	const podID = "pod_id"

	for tName, tCase := range map[string]func(ctx context.Context, env evergreen.Environment, t *testing.T){
		"SucceedsWithSomePodsRemaining": func(ctx context.Context, env evergreen.Environment, t *testing.T) {
			pd := NewPodDispatcher("group_id", nil, []string{podID, "other_pod_id", "another_pod_id"})
			require.NoError(t, pd.Insert(t.Context()))
			require.NoError(t, pd.RemovePod(ctx, env, podID))

			dbDisp, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Empty(t, dbDisp.TaskIDs)
			assert.ElementsMatch(t, dbDisp.PodIDs, []string{"other_pod_id", "another_pod_id"})
		},
		"SucceedsWhenTheLastPodIsBeingRemovedWithoutAnyTasks": func(ctx context.Context, env evergreen.Environment, t *testing.T) {
			pd := NewPodDispatcher("group_id", nil, []string{podID})
			require.NoError(t, pd.Insert(t.Context()))

			require.NoError(t, pd.RemovePod(ctx, env, podID))

			dbDisp, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Empty(t, dbDisp.TaskIDs)
			assert.Empty(t, dbDisp.PodIDs)
		},
		"SucceedsAndFixesTasksWhenTheLastPodIsBeingRemoved": func(ctx context.Context, env evergreen.Environment, t *testing.T) {
			t0 := task.Task{
				Id:                     "task_id0",
				ExecutionPlatform:      task.ExecutionPlatformContainer,
				Status:                 evergreen.TaskUndispatched,
				Activated:              true,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
				PodID:                  podID,
				Project:                "project-ref",
			}
			require.NoError(t, t0.Insert(t.Context()))

			v := model.Version{
				Id:     "version_id",
				Status: evergreen.BuildStarted,
			}
			require.NoError(t, v.Insert(t.Context()))
			pRef := model.ProjectRef{
				Identifier: "project-ref",
			}
			require.NoError(t, pRef.Insert(t.Context()))
			pp := model.ParserProject{
				Id: v.Id,
			}
			require.NoError(t, pp.Insert(t.Context()))
			b := build.Build{
				Id:      "build_id",
				Version: v.Id,
			}
			require.NoError(t, b.Insert(t.Context()))
			t1 := task.Task{
				Id:                          "task_id1",
				BuildId:                     b.Id,
				Version:                     v.Id,
				ExecutionPlatform:           task.ExecutionPlatformContainer,
				Status:                      evergreen.TaskUndispatched,
				Activated:                   true,
				ContainerAllocated:          true,
				ContainerAllocatedTime:      time.Now(),
				ContainerAllocationAttempts: 100,
				PodID:                       podID,
			}
			require.NoError(t, t1.Insert(t.Context()))

			pd := NewPodDispatcher("group_id", []string{t0.Id, t1.Id}, []string{podID})
			require.NoError(t, pd.Insert(t.Context()))

			require.NoError(t, pd.RemovePod(ctx, env, podID))

			dbDisp, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Empty(t, dbDisp.TaskIDs)
			assert.Empty(t, dbDisp.PodIDs)

			dbTask0, err := task.FindOneId(ctx, t0.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask0)
			assert.False(t, dbTask0.ContainerAllocated)
			assert.True(t, dbTask0.ShouldAllocateContainer(), "task should be able to allocate another container")

			dbTask1, err := task.FindOneId(ctx, t1.Id)
			require.NoError(t, err)
			require.NotZero(t, dbTask1)
			assert.False(t, dbTask1.ShouldAllocateContainer(), "task should not be able to allocate another container because it has no remaining attempts")
			assert.True(t, dbTask1.IsFinished(), "task should be finished because it has used up all of its container allocation attempts")
			assert.False(t, dbTask1.ContainerAllocated)

			dbBuild, err := build.FindOneId(t.Context(), b.Id)
			require.NoError(t, err)
			require.NotZero(t, dbBuild)
			assert.True(t, dbBuild.IsFinished(), "build should be updated after its task is finished")

			dbVersion, err := model.VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.VersionFailed, dbVersion.Status, "version should be updated after its task is finished")
		},
		"FailsWhenPodIsNotInTheDispatcher": func(ctx context.Context, env evergreen.Environment, t *testing.T) {
			pd := NewPodDispatcher("group_id", []string{"task_id"}, []string{"pod_id"})
			require.NoError(t, pd.Insert(t.Context()))

			assert.Error(t, pd.RemovePod(ctx, env, "nonexistent"))

			dbDisp, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Equal(t, []string{"task_id"}, pd.TaskIDs)
			assert.Equal(t, []string{"pod_id"}, pd.PodIDs)
		},
		"FailsWhenDBDispatcherIsModified": func(ctx context.Context, env evergreen.Environment, t *testing.T) {
			const modCount = 10

			tsk := task.Task{
				Id:                     "task_id",
				ExecutionPlatform:      task.ExecutionPlatformContainer,
				Status:                 evergreen.TaskUndispatched,
				Activated:              true,
				ContainerAllocated:     true,
				ContainerAllocatedTime: time.Now(),
				PodID:                  podID,
			}

			pd := NewPodDispatcher("group_id", []string{tsk.Id}, []string{podID})
			pd.ModificationCount = modCount
			require.NoError(t, pd.Insert(t.Context()))
			pd.ModificationCount = 0

			assert.Error(t, pd.RemovePod(ctx, env, podID))

			dbDisp, err := FindOneByID(t.Context(), pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDisp)
			assert.Equal(t, []string{tsk.Id}, pd.TaskIDs)
			assert.Equal(t, []string{podID}, pd.PodIDs)
			assert.Equal(t, modCount, dbDisp.ModificationCount)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer tcancel()

			require.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, build.Collection, model.VersionCollection))

			tskPod := pod.Pod{
				ID: podID,
			}
			assert.NoError(t, tskPod.Insert(t.Context()))
			tCase(tctx, env, t)
		})
	}
}
