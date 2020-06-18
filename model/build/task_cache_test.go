package build

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateCachedTask(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(Collection), "Error clearing build collection")
	b := &Build{
		Id: "build1",
		Tasks: []TaskCache{
			{
				Id:        "task1",
				Status:    evergreen.TaskUndispatched,
				TimeTaken: 0,
			},
		},
	}
	assert.NoError(b.Insert())

	//test that invalid inputs error
	assert.Error(UpdateCachedTask(nil, 0))
	assert.Error(UpdateCachedTask(&task.Task{Id: "", BuildId: "build1", Status: evergreen.TaskUndispatched}, 0))
	assert.Error(UpdateCachedTask(&task.Task{Id: "task1", BuildId: "", Status: evergreen.TaskUndispatched}, 0))
	assert.Error(UpdateCachedTask(&task.Task{Id: "task1", BuildId: "build1", Status: ""}, 0))

	// test that status updates work correctly
	assert.NoError(UpdateCachedTask(&task.Task{Id: "task1", BuildId: "build1", Status: evergreen.TaskDispatched}, 0))
	dbBuild, err := FindOne(ById(b.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Equal(evergreen.TaskDispatched, dbBuild.Tasks[0].Status)

	// test that failure details updates work correctly
	assert.NoError(UpdateCachedTask(&task.Task{
		Id:      "task1",
		BuildId: "build1",
		Status:  evergreen.TaskFailed,
		Details: apimodels.TaskEndDetail{Status: evergreen.TaskFailed, Type: evergreen.CommandTypeSystem},
	}, 0))
	dbBuild, err = FindOne(ById(b.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Equal(evergreen.CommandTypeSystem, dbBuild.Tasks[0].StatusDetails.Type)

	// test that incrementing time taken works correctly
	assert.NoError(UpdateCachedTask(&task.Task{Id: "task1", BuildId: "build1", Status: evergreen.TaskSucceeded}, 1*time.Second))
	assert.NoError(UpdateCachedTask(&task.Task{Id: "task1", BuildId: "build1", Status: evergreen.TaskSucceeded}, 3*time.Second))
	dbBuild, err = FindOne(ById(b.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Equal(4*time.Second, dbBuild.Tasks[0].TimeTaken)

	// test blocked
	assert.NoError(UpdateCachedTask(&task.Task{
		Id:        "task1",
		BuildId:   "build1",
		Status:    evergreen.TaskUndispatched,
		DependsOn: []task.Dependency{{TaskId: "task2", Unattainable: true}, {TaskId: "task1", Unattainable: false}},
	}, 0))
	dbBuild, err = FindOne(ById(b.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.True(dbBuild.Tasks[0].Blocked)
}

func TestSetManyCachedTasksActivated(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	b := Build{
		Id: "b0",
		Tasks: []TaskCache{
			{Id: "t0", Activated: false},
			{Id: "t1", Activated: false},
		},
	}
	require.NoError(t, b.Insert())

	assert.NoError(t, SetManyCachedTasksActivated([]task.Task{{Id: "t0", BuildId: "b0"}}, true))

	bDb, err := FindOneId("b0")
	require.NoError(t, err)
	assert.True(t, bDb.Tasks[0].Activated)
	assert.False(t, bDb.Tasks[1].Activated)
}
