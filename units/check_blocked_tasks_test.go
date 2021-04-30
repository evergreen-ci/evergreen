package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestCheckUnmarkedBlockingTasksWithDeactivatedTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection, build.Collection))
	t1 := &task.Task{
		Id:        "t1",
		BuildId:   "b1",
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId: "t2",
				Status: evergreen.TaskSucceeded,
			},
		},
	}
	t2 := &task.Task{
		Id:        "t2",
		BuildId:   "b1",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	}
	b1 := build.Build{
		Id: "b1",
	}
	assert.NoError(t, t1.Insert())
	assert.NoError(t, t2.Insert())
	assert.NoError(t, b1.Insert())
	depCache := map[string]task.Task{}
	modified, err := checkUnmarkedBlockingTasks(t1, depCache)
	assert.NoError(t, err)
	assert.Equal(t, modified, 1)
	t1FromDb, err := task.FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, t1FromDb)
	assert.False(t, t1FromDb.Activated)
}

func TestCheckUnmarkedBlockingTasksWithBlockedTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection, build.Collection))
	t1 := &task.Task{
		Id:        "t1",
		BuildId:   "b1",
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId:       "t2",
				Status:       evergreen.TaskSucceeded,
				Unattainable: false,
			},
		},
	}
	t2 := &task.Task{
		Id:        "t2",
		BuildId:   "b1",
		Activated: true,
		Status:    evergreen.TaskUndispatched,
		DependsOn: []task.Dependency{
			{
				TaskId:       "t3",
				Status:       evergreen.TaskSucceeded,
				Unattainable: true,
			},
		},
	}
	t3 := &task.Task{
		Id:        "t3",
		Status:    evergreen.TaskFailed,
		Activated: true,
	}
	b1 := build.Build{
		Id: "b1",
		Tasks: []build.TaskCache{
			{Id: "t1"},
			{Id: "t2"},
			{Id: "t3"},
		},
	}
	assert.NoError(t, t1.Insert())
	assert.NoError(t, t2.Insert())
	assert.NoError(t, t3.Insert())
	assert.NoError(t, b1.Insert())
	depCache := map[string]task.Task{}

	modified, err := checkUnmarkedBlockingTasks(t1, depCache)
	assert.NoError(t, err)
	assert.Equal(t, modified, 2)

	t1FromDb, err := task.FindOneId("t1")
	assert.NoError(t, err)
	assert.NotNil(t, t1FromDb)
	assert.False(t, t1FromDb.Activated)
}
