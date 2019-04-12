package build

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestUpdateCachedTask(t *testing.T) {
	assert := assert.New(t)
	testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing build collection")
	b := &Build{
		Id: "build1",
		Tasks: []TaskCache{
			TaskCache{
				Id:        "task1",
				Status:    evergreen.TaskUndispatched,
				TimeTaken: 0,
			},
		},
	}
	assert.NoError(b.Insert())

	//test that invalid inputs error
	assert.Error(UpdateCachedTask("", b.Tasks[0].Id, evergreen.TaskDispatched, 0))
	assert.Error(UpdateCachedTask(b.Id, "", evergreen.TaskDispatched, 0))
	assert.Error(UpdateCachedTask(b.Id, b.Tasks[0].Id, "", 0))

	// test that status updates work correctly
	assert.NoError(UpdateCachedTask(b.Id, b.Tasks[0].Id, evergreen.TaskDispatched, 0))
	dbBuild, err := FindOne(ById(b.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Equal(evergreen.TaskDispatched, dbBuild.Tasks[0].Status)

	// test that incrementing time taken works correctly
	assert.NoError(UpdateCachedTask(b.Id, b.Tasks[0].Id, evergreen.TaskSucceeded, 1*time.Second))
	assert.NoError(UpdateCachedTask(b.Id, b.Tasks[0].Id, evergreen.TaskSucceeded, 3*time.Second))
	dbBuild, err = FindOne(ById(b.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Equal(4*time.Second, dbBuild.Tasks[0].TimeTaken)
}
