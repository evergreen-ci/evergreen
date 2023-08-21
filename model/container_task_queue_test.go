package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerTaskQueue(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, ProjectRefCollection))
	}()
	getTaskThatNeedsContainerAllocation := func() task.Task {
		return task.Task{
			Id:                 utility.RandomString(),
			Activated:          true,
			ActivatedTime:      time.Now(),
			Status:             evergreen.TaskUndispatched,
			ContainerAllocated: false,
			ExecutionPlatform:  task.ExecutionPlatformContainer,
		}
	}
	getProjectRef := func() ProjectRef {
		return ProjectRef{
			Id:         utility.RandomString(),
			Identifier: utility.RandomString(),
			Enabled:    true,
		}
	}
	checkEmpty := func(t *testing.T, ctq *ContainerTaskQueue) {
		require.Zero(t, ctq.Len())
		require.False(t, ctq.HasNext())
		require.Zero(t, ctq.Next())
	}
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsAllTasksThatNeedContainerAllocationInOrderOfActivation": func(t *testing.T) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())

			needsAllocation0 := getTaskThatNeedsContainerAllocation()
			needsAllocation0.Project = ref.Id
			require.NoError(t, needsAllocation0.Insert())

			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.ContainerAllocated = true
			require.NoError(t, doesNotNeedAllocation.Insert())

			needsAllocation1 := getTaskThatNeedsContainerAllocation()
			needsAllocation1.Project = ref.Id
			needsAllocation1.ActivatedTime = time.Now().Add(-time.Hour)
			require.NoError(t, needsAllocation1.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			assert.Equal(t, 2, ctq.Len())
			assert.True(t, ctq.HasNext())

			first := ctq.Next()
			require.NotZero(t, first)
			assert.Equal(t, needsAllocation1.Id, first.Id, "should return task in need of allocation with earlier activation time")

			assert.Equal(t, 1, ctq.Len())
			assert.True(t, ctq.HasNext())

			second := ctq.Next()
			require.NotZero(t, second)
			assert.Equal(t, needsAllocation0.Id, second.Id, "should return task in need of alloation with later activation time")

			checkEmpty(t, ctq)
		},
		"SetsFirstTaskScheduledTime": func(t *testing.T) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			assert.Equal(t, 1, ctq.Len())
			assert.True(t, ctq.HasNext())

			first := ctq.Next()
			require.NotZero(t, first)
			assert.Equal(t, needsAllocation.Id, first.Id, "should return task in need of allocation")

			dbFirstTask, err := task.FindOneId(first.Id)
			require.NoError(t, err)
			require.NotZero(t, dbFirstTask)
			assert.False(t, utility.IsZeroTime(dbFirstTask.ScheduledTime))

			checkEmpty(t, ctq)
		},
		"ReturnsNoTask": func(t *testing.T) {
			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)
			require.NotZero(t, ctq)

			checkEmpty(t, ctq)
		},
		"DoesNotReturnTaskMissingProject": func(t *testing.T) {
			needsAllocation := getTaskThatNeedsContainerAllocation()
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			checkEmpty(t, ctq)
		},
		"DoesNotReturnTaskWithInvalidProject": func(t *testing.T) {
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = "foo"
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			checkEmpty(t, ctq)
		},
		"DoesNotReturnTaskWithProjectThatDisabledTaskDispatching": func(t *testing.T) {
			ref := getProjectRef()
			ref.DispatchingDisabled = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			checkEmpty(t, ctq)
		},
		"ReturnsPatchTaskWithProjectThatEnabledPatching": func(t *testing.T) {
			ref := getProjectRef()
			ref.PatchingDisabled = utility.FalsePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.PatchVersionRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			assert.Equal(t, 1, ctq.Len())
			assert.True(t, ctq.HasNext())

			tsk := ctq.Next()
			require.NotZero(t, tsk)
			assert.Equal(t, needsAllocation.Id, tsk.Id)

			checkEmpty(t, ctq)
		},
		"DoesNotReturnPatchTaskWithProjectThatDisabledPatching": func(t *testing.T) {
			ref := getProjectRef()
			ref.PatchingDisabled = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.PatchVersionRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			checkEmpty(t, ctq)
		},
		"DoesNotReturnTaskWithDisabledProject": func(t *testing.T) {
			ref := getProjectRef()
			ref.Enabled = false
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			checkEmpty(t, ctq)
		},
		"ReturnsGitHubTaskInDisabledAndHiddenProject": func(t *testing.T) {
			ref := getProjectRef()
			ref.Enabled = false
			ref.Hidden = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.GithubPRRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			assert.Equal(t, 1, ctq.Len())
			assert.True(t, ctq.HasNext())

			tsk := ctq.Next()
			require.NotZero(t, tsk)
			assert.Equal(t, needsAllocation.Id, tsk.Id)

			checkEmpty(t, ctq)
		},
		"DoesNotReturnNonGitHubTaskInDisabledAndHiddenProject": func(t *testing.T) {
			ref := getProjectRef()
			ref.Enabled = false
			ref.Hidden = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.PatchVersionRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			ctq, err := NewContainerTaskQueue()
			require.NoError(t, err)

			checkEmpty(t, ctq)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, ProjectRefCollection))
			tCase(t)
		})
	}
}
