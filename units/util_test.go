package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestHandlePoisonedHost(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(host.Collection, task.Collection))

	t1 := &task.Task{
		Id:     "t1",
		Status: evergreen.TaskStarted,
	}
	assert.NoError(t1.Insert())

	parent := host.Host{
		Id:            "parent",
		HasContainers: true,
		Status:        evergreen.HostRunning,
	}
	container1 := host.Host{
		Id:       "container1",
		Status:   evergreen.HostRunning,
		ParentID: parent.Id,
	}
	container2 := host.Host{
		Id:          "container2",
		Status:      evergreen.HostRunning,
		ParentID:    parent.Id,
		RunningTask: t1.Id,
	}
	assert.NoError(parent.Insert())
	assert.NoError(container1.Insert())
	assert.NoError(container2.Insert())

	env := evergreen.GetEnvironment()
	ctx := context.Background()

	_ = HandlePoisonedHost(ctx, env, &container1, "")
	dbParent, err := host.FindOneId(parent.Id)
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, dbParent.Status)
	dbContainer1, err := host.FindOneId(container1.Id)
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, dbContainer1.Status)
	dbContainer2, err := host.FindOneId(container2.Id)
	assert.NoError(err)
	assert.Equal(evergreen.HostDecommissioned, dbContainer2.Status)

	t1, err = task.FindOneId(t1.Id)
	assert.NoError(err)
	assert.Equal(evergreen.TaskFailed, t1.Status)
}
