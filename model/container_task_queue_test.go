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

func TestContainerTaskQueueNext(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, ProjectRefCollection))
	}()
	getTaskThatNeedsContainerAllocation := func() task.Task {
		return task.Task{
			Id:                utility.RandomString(),
			Activated:         true,
			ActivatedTime:     time.Now(),
			Status:            evergreen.TaskContainerUnallocated,
			ExecutionPlatform: task.ExecutionPlatformContainer,
		}
	}
	getProjectRef := func() ProjectRef {
		return ProjectRef{
			Id:         utility.RandomString(),
			Identifier: utility.RandomString(),
			Enabled:    utility.TruePtr(),
		}
	}
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsAllTasksThatNeedContainerAllocationInOrderOfActivation": func(t *testing.T) {
			ref := getProjectRef()
			require.NoError(t, ref.Insert())

			needsAllocation0 := getTaskThatNeedsContainerAllocation()
			needsAllocation0.Project = ref.Id
			require.NoError(t, needsAllocation0.Insert())

			doesNotNeedAllocation := getTaskThatNeedsContainerAllocation()
			doesNotNeedAllocation.Status = evergreen.TaskContainerAllocated
			require.NoError(t, doesNotNeedAllocation.Insert())

			needsAllocation1 := getTaskThatNeedsContainerAllocation()
			needsAllocation1.Project = ref.Id
			needsAllocation1.ActivatedTime = time.Now().Add(-time.Hour)
			require.NoError(t, needsAllocation1.Insert())

			var ctq ContainerTaskQueue

			first, err := ctq.Next()
			require.NoError(t, err)
			require.NotZero(t, first)
			assert.Equal(t, needsAllocation1.Id, first.Id, "should return task in need of allocation with earlier activation time")

			second, err := ctq.Next()
			require.NoError(t, err)
			require.NotZero(t, first)
			assert.Equal(t, needsAllocation0.Id, second.Id, "should return task in need of alloation with later activation time")

			third, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, third)
		},
		"ReturnsNoTask": func(t *testing.T) {
			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
		"DoesNotReturnTaskMissingProject": func(t *testing.T) {
			needsAllocation := getTaskThatNeedsContainerAllocation()
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
		"DoesNotReturnTaskWithInvalidProject": func(t *testing.T) {
			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = "foo"
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
		"DoesNotReturnTaskWithProjectThatDisabledTaskDispatching": func(t *testing.T) {
			ref := getProjectRef()
			ref.DispatchingDisabled = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
		"ReturnsPatchTaskWithProjectThatEnabledPatching": func(t *testing.T) {
			ref := getProjectRef()
			ref.PatchingDisabled = utility.FalsePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.PatchVersionRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			require.NotZero(t, tsk)
			assert.Equal(t, needsAllocation.Id, tsk.Id)
		},
		"DoesNotReturnPatchTaskWithProjectThatDisabledPatching": func(t *testing.T) {
			ref := getProjectRef()
			ref.PatchingDisabled = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.PatchVersionRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
		"DoesNotReturnTaskWithDisabledProject": func(t *testing.T) {
			ref := getProjectRef()
			ref.Enabled = utility.FalsePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
		"ReturnsGitHubTaskInDisabledAndHiddenProject": func(t *testing.T) {
			ref := getProjectRef()
			ref.Enabled = utility.FalsePtr()
			ref.Hidden = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.GithubPRRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			require.NotZero(t, tsk)
			assert.Equal(t, needsAllocation.Id, tsk.Id)
		},
		"DoesNotReturnNonGitHubTaskInDisabledAndHiddenProject": func(t *testing.T) {
			ref := getProjectRef()
			ref.Enabled = utility.FalsePtr()
			ref.Hidden = utility.TruePtr()
			require.NoError(t, ref.Insert())

			needsAllocation := getTaskThatNeedsContainerAllocation()
			needsAllocation.Requester = evergreen.PatchVersionRequester
			needsAllocation.Project = ref.Id
			require.NoError(t, needsAllocation.Insert())

			var ctq ContainerTaskQueue
			tsk, err := ctq.Next()
			require.NoError(t, err)
			assert.Zero(t, tsk)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, ProjectRefCollection))
			tCase(t)
		})
	}
}
