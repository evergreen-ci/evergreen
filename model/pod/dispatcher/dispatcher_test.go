package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
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
			change, err := pd.UpsertAtomically()
			require.NoError(t, err)
			require.Equal(t, change.Updated, 1)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
		},
		"UpdatesExistingPodDispatcher": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			change, err := pd.UpsertAtomically()
			require.NoError(t, err)
			require.Equal(t, change.Updated, 1)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
			assert.NotZero(t, pd.ModificationCount)
			assert.Equal(t, pd.ModificationCount, dbDispatcher.ModificationCount)
		},
		"FailsWithMatchingGroupIDButDifferentDispatcherID": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			modified := pd
			modified.ID = primitive.NewObjectID().Hex()
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically()
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(modified.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbDispatcher)

			dbDispatcher, err = FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
		"FailsWithMatchingDispatcherIDButDifferentGroupID": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			modified := pd
			modified.GroupID = utility.RandomString()
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically()
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
		"FailsWithDifferentModificationCount": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			modified := pd
			modified.ModificationCount = 12345
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically()
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(pd.ID)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, event.AllLogCollection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	getDispatchableTask := func() task.Task {
		return task.Task{
			Id:                utility.RandomString(),
			Activated:         true,
			ActivatedTime:     time.Now(),
			Status:            evergreen.TaskContainerAllocated,
			ExecutionPlatform: task.ExecutionPlatformContainer,
			DisplayTaskId:     utility.ToStringPtr(""),
		}
	}
	getProjectRef := func() model.ProjectRef {
		return model.ProjectRef{
			Id:         utility.RandomString(),
			Identifier: utility.RandomString(),
			Enabled:    utility.TruePtr(),
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
		dbTask, err := task.FindOneId(tsk.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.Equal(t, evergreen.TaskDispatched, dbTask.Status)
		assert.False(t, utility.IsZeroTime(dbTask.DispatchTime))
		assert.False(t, utility.IsZeroTime(dbTask.LastHeartbeat))
		assert.Equal(t, p.AgentVersion, dbTask.AgentVersion)

		dbPod, err := pod.FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, tsk.Id, dbPod.RunningTask)

		taskEvents, err := event.FindAllByResourceID(dbTask.Id)
		require.NoError(t, err)
		require.Len(t, taskEvents, 1)
		assert.Equal(t, event.TaskDispatched, taskEvents[0].EventType)

		podEvents, err := event.FindAllByResourceID(p.ID)
		require.NoError(t, err)
		require.Len(t, podEvents, 1)
		assert.Equal(t, string(event.EventPodAssignedTask), podEvents[0].EventType)
	}

	checkTaskUnallocated := func(t *testing.T, tsk task.Task) {
		dbTask, err := task.FindOneId(tsk.Id)
		require.NoError(t, err)
		require.NotZero(t, dbTask)
		assert.Equal(t, evergreen.TaskContainerUnallocated, dbTask.Status)
	}

	checkDispatcherTasks := func(t *testing.T, pd PodDispatcher, taskIDs []string) {
		dbDispatcher, err := FindOneByID(pd.ID)
		require.NoError(t, err)
		require.NotZero(t, dbDispatcher)

		require.Len(t, pd.TaskIDs, len(taskIDs))
		for i := range taskIDs {
			assert.Equal(t, taskIDs[i], pd.TaskIDs[i])
		}
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, params testCaseParams){
		"DispatchesTask": func(ctx context.Context, t *testing.T, params testCaseParams) {
			require.NoError(t, params.dispatcher.Insert())
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, params.task.Id, nextTask.Id)

			checkTaskDispatchedToPod(t, params.task, params.pod)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DispatchesTaskInDisabledHiddenProject": func(ctx context.Context, t *testing.T, params testCaseParams) {
			params.task.Requester = evergreen.GithubPRRequester
			params.ref.Enabled = utility.FalsePtr()
			params.ref.Hidden = utility.TruePtr()
			require.NoError(t, params.dispatcher.Insert())
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, params.task.Id, nextTask.Id)

			checkTaskDispatchedToPod(t, params.task, params.pod)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesNonexistentTaskAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			require.NoError(t, params.dispatcher.Insert())
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.ref.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			dbTask, err := task.FindOneId(params.task.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbTask)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesTaskWithNonexistentProjectAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.dispatcher.Insert())
			require.NoError(t, params.pod.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesDeactivatedTaskAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			params.task.Activated = false
			require.NoError(t, params.dispatcher.Insert())
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesUndispatchableTaskAndReturnsNextDispatchableTask": func(ctx context.Context, t *testing.T, params testCaseParams) {
			params.task.Activated = false
			dispatchableTask0 := getDispatchableTask()
			dispatchableTask0.Project = params.ref.Id
			dispatchableTask1 := getDispatchableTask()
			dispatchableTask1.Project = params.ref.Id
			params.dispatcher.TaskIDs = append(params.dispatcher.TaskIDs, dispatchableTask0.Id, dispatchableTask1.Id)
			require.NoError(t, params.dispatcher.Insert())
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, dispatchableTask0.Insert())
			require.NoError(t, dispatchableTask1.Insert())
			require.NoError(t, params.ref.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			require.NoError(t, err)
			require.NotZero(t, nextTask)
			assert.Equal(t, dispatchableTask0.Id, nextTask.Id)

			checkTaskUnallocated(t, params.task)
			checkTaskDispatchedToPod(t, dispatchableTask0, params.pod)
			checkDispatcherTasks(t, params.dispatcher, []string{dispatchableTask1.Id})
		},
		"DequeuesTaskWithUndispatchableStatusAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			require.NoError(t, params.pod.Insert())
			params.task.Status = evergreen.TaskContainerUnallocated
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())
			require.NoError(t, params.dispatcher.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesDisabledTaskAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			require.NoError(t, params.pod.Insert())
			params.task.Priority = evergreen.DisabledTaskPriority
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())
			require.NoError(t, params.dispatcher.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesTaskInDisabledProjectAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			params.ref.Enabled = utility.FalsePtr()
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())
			require.NoError(t, params.dispatcher.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"DequeuesTaskInProjectWithDispatchingDisabledAndDoesNotDispatchIt": func(ctx context.Context, t *testing.T, params testCaseParams) {
			params.ref.DispatchingDisabled = utility.TruePtr()
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())
			require.NoError(t, params.dispatcher.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.NoError(t, err)
			assert.Zero(t, nextTask)

			checkTaskUnallocated(t, params.task)
			checkDispatcherTasks(t, params.dispatcher, nil)
		},
		"FailsWithPodInStateThatCannotRunTasks": func(ctx context.Context, t *testing.T, params testCaseParams) {
			params.pod.Status = pod.StatusDecommissioned
			require.NoError(t, params.pod.Insert())
			require.NoError(t, params.task.Insert())
			require.NoError(t, params.ref.Insert())
			require.NoError(t, params.dispatcher.Insert())

			nextTask, err := params.dispatcher.AssignNextTask(ctx, params.env, &params.pod)
			assert.Error(t, err)
			assert.Zero(t, nextTask)

			checkDispatcherTasks(t, params.dispatcher, []string{params.task.Id})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()
			require.NoError(t, db.ClearCollections(Collection, pod.Collection, task.Collection, event.AllLogCollection))

			p := pod.Pod{
				ID:           utility.RandomString(),
				Status:       pod.StatusRunning,
				AgentVersion: evergreen.AgentVersion,
			}
			tsk := getDispatchableTask()
			ref := getProjectRef()
			tsk.Project = ref.Id
			pd := NewPodDispatcher(GetGroupID(&tsk), []string{tsk.Id}, []string{p.ID})

			tCase(tctx, t, testCaseParams{
				env:        env,
				dispatcher: pd,
				pod:        p,
				task:       tsk,
				ref:        ref,
			})
		})
	}
}
