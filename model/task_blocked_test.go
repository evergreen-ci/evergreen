package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockedState(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	t1 := &task.Task{
		Id: "t1",
		DependsOn: []task.Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert())
	t2 := &task.Task{
		Id:     "t2",
		Status: evergreen.TaskFailed,
		DependsOn: []task.Dependency{
			{TaskId: "t3", Status: evergreen.TaskFailed},
		},
	}
	assert.NoError(t2.Insert())
	t3 := &task.Task{
		Id:     "t3",
		Status: evergreen.TaskUnstarted,
		DependsOn: []task.Dependency{
			{TaskId: "t4", Status: AllStatuses},
		},
	}
	assert.NoError(t3.Insert())
	t4 := &task.Task{
		Id:     "t4",
		Status: evergreen.TaskSucceeded,
	}
	assert.NoError(t4.Insert())

	state, err := BlockedState(t4, nil)
	assert.NoError(err)
	assert.Equal(taskRunnable, state)
	state, err = BlockedState(t3, nil)
	assert.NoError(err)
	assert.Equal(taskRunnable, state)
	state, err = BlockedState(t2, nil)
	assert.NoError(err)
	assert.Equal(taskPending, state)
	state, err = BlockedState(t1, nil)
	assert.NoError(err)
	assert.Equal(taskBlocked, state)
}

func TestCircularDependency(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	t1 := &task.Task{
		Id:          "t1",
		DisplayName: "t1",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []task.Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert())
	t2 := task.Task{
		Id:          "t2",
		DisplayName: "t2",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []task.Dependency{
			{TaskId: "t1", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t2.Insert())
	assert.NotPanics(func() {
		err := CircularDependencies(t1)
		assert.Contains(err.Error(), "Dependency cycle detected")
	})
}

func TestSiblingDependency(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection))
	t1 := &task.Task{
		Id:          "t1",
		DisplayName: "t1",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []task.Dependency{
			{TaskId: "t2", Status: evergreen.TaskSucceeded},
			{TaskId: "t3", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t1.Insert())
	t2 := task.Task{
		Id:          "t2",
		DisplayName: "t2",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
		DependsOn: []task.Dependency{
			{TaskId: "t4", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t2.Insert())
	t3 := task.Task{
		Id:          "t3",
		DisplayName: "t3",
		Activated:   true,
		Status:      evergreen.TaskStarted,
		DependsOn: []task.Dependency{
			{TaskId: "t4", Status: evergreen.TaskSucceeded},
		},
	}
	assert.NoError(t3.Insert())
	t4 := task.Task{
		Id:          "t4",
		DisplayName: "t4",
		Activated:   true,
		Status:      evergreen.TaskSucceeded,
	}
	assert.NoError(t4.Insert())
	state, err := BlockedState(t1, nil)
	assert.NoError(err)
	assert.Equal(taskPending, state)
}

func TestAllTasksFinished(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.ClearCollections(task.Collection), "error clearing collection")
	b := build.Build{Id: "b1", Activated: true}
	tasks := []task.Task{
		{
			Id:        "t1",
			BuildId:   "b1",
			Status:    evergreen.TaskStarted,
			Activated: true,
		},
		{
			Id:        "t2",
			BuildId:   "b1",
			Activated: true,
			Status:    evergreen.TaskStarted,
		},
		{
			Id:        "t3",
			BuildId:   "b1",
			Status:    evergreen.TaskStarted,
			Activated: true,
		},
		{
			Id:        "t4",
			BuildId:   "b1",
			Status:    evergreen.TaskStarted,
			Activated: true,
		},
		// this task is unscheduled
		{
			Id:      "t5",
			BuildId: "b1",
			Status:  evergreen.TaskUndispatched,
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert())
	}

	assert.False(AllUnblockedTasksFinished(b, nil))

	assert.NoError(tasks[0].MarkFailed())
	assert.False(AllUnblockedTasksFinished(b, nil))

	assert.NoError(tasks[1].MarkFailed())
	assert.False(AllUnblockedTasksFinished(b, nil))

	assert.NoError(tasks[2].MarkFailed())
	assert.False(AllUnblockedTasksFinished(b, nil))

	assert.NoError(tasks[3].MarkFailed())
	assert.True(AllUnblockedTasksFinished(b, nil))

	// Only one activated task
	require.NoError(t, db.ClearCollections(task.Collection), "error clearing collection")
	tasks = []task.Task{
		{
			Id:          "t1",
			BuildId:     "b1",
			DisplayName: "compile",
			Status:      evergreen.TaskStarted,
			Activated:   true,
		},
		{
			Id:      "t2",
			BuildId: "b1",
			Status:  evergreen.TaskStarted,
		},
		{
			Id:          "t3",
			BuildId:     "b1",
			DisplayName: evergreen.PushStage,
			Status:      evergreen.TaskStarted,
		},
	}
	for _, task := range tasks {
		assert.NoError(task.Insert())
	}
	assert.False(AllUnblockedTasksFinished(b, nil))
	assert.NoError(tasks[0].MarkFailed())
	assert.True(AllUnblockedTasksFinished(b, nil))

	// Build is finished
	require.NoError(t, db.ClearCollections(task.Collection), "error clearing collection")
	task1 := task.Task{
		Id:        "t0",
		BuildId:   "b1",
		Status:    evergreen.TaskFailed,
		Activated: false,
	}
	assert.NoError(task1.Insert())
	complete, status, err := AllUnblockedTasksFinished(b, nil)
	assert.NoError(err)
	assert.True(complete)
	assert.Equal(status, evergreen.BuildFailed)

	// Display task
	require.NoError(t, db.ClearCollections(task.Collection), "error clearing collection")
	t0 := task.Task{
		Id:      "t0",
		BuildId: "b1",
		Status:  evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{
			Status: evergreen.TaskFailed,
			Type:   "test",
		},
	}
	t1 := task.Task{
		Id:      "t1",
		BuildId: "b1",
		Status:  evergreen.TaskUndispatched,
		DependsOn: []task.Dependency{
			{
				TaskId: t0.Id,
				Status: evergreen.TaskSucceeded,
			},
		},
	}
	d0 := task.Task{
		Id:             "d0",
		BuildId:        "b1",
		Status:         evergreen.TaskStarted,
		DisplayOnly:    true,
		ExecutionTasks: []string{"e0", "e1"},
	}
	e0 := task.Task{
		Id:      "e0",
		BuildId: "b1",
		Status:  evergreen.TaskFailed,
	}
	e1 := task.Task{
		Id:      "e1",
		BuildId: "b1",
		DependsOn: []task.Dependency{
			{
				TaskId: e0.Id,
				Status: evergreen.TaskSucceeded,
			},
		},
		Status: evergreen.TaskUndispatched,
	}

	assert.NoError(t0.Insert())
	assert.NoError(t1.Insert())
	assert.NoError(d0.Insert())
	assert.NoError(e0.Insert())
	assert.NoError(e1.Insert())
	complete, _, err = AllUnblockedTasksFinished(b, nil)
	assert.NoError(err)
	assert.True(complete)

	// inactive build should not be complete
	b.Activated = false
	complete, _, err = AllUnblockedTasksFinished(b, nil)
	assert.NoError(err)
	assert.False(complete)
}
