package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
)

func TestCompareTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(distro.Collection, task.Collection, model.VersionCollection))
	distroId := "d"
	t1 := task.Task{
		Id:        "t1",
		DistroId:  distroId,
		Priority:  1,
		Version:   "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(t, t1.Insert())
	t2 := task.Task{
		Id:        "t2",
		DistroId:  distroId,
		Priority:  2,
		Version:   "v2",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(t, t2.Insert())
	t3 := task.Task{
		Id:           "t3",
		DistroId:     distroId,
		Priority:     2,
		Version:      "v2",
		GenerateTask: true,
		Requester:    evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(t, t3.Insert())
	cqTask := task.Task{
		Id:        "cqt1",
		DistroId:  distroId,
		Priority:  0,
		Version:   "cqv1",
		Requester: evergreen.MergeTestRequester,
	}
	assert.NoError(t, cqTask.Insert())
	v1 := model.Version{
		Id:        "v1",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(t, v1.Insert())
	v2 := model.Version{
		Id:        "v2",
		Requester: evergreen.RepotrackerVersionRequester,
	}
	assert.NoError(t, v2.Insert())
	cqVersion := model.Version{
		Id:        "cqv1",
		Requester: evergreen.MergeTestRequester,
	}
	assert.NoError(t, cqVersion.Insert())

	order, logic, err := CompareTasks(ctx, []string{"t1", "t2"}, true)
	assert.NoError(t, err)
	assert.Equal(t, []string{"t2", "t1"}, order)
	assert.Equal(t, "task priority: higher is first", logic[t1.Id][t2.Id])

	order, logic, err = CompareTasks(ctx, []string{"t1", "t2", "t3"}, true)
	assert.NoError(t, err)
	assert.Equal(t, []string{"t3", "t2", "t1"}, order)
	assert.Equal(t, "task generator: higher task is a generator", logic[t2.Id][t3.Id])
	assert.Equal(t, "task priority: higher is first", logic[t1.Id][t2.Id])

	order, logic, err = CompareTasks(ctx, []string{"t1", "t2", "t3", "cqt1"}, true)
	assert.NoError(t, err)
	assert.Equal(t, []string{"cqt1", "t3", "t2", "t1"}, order)
	//TODO: change the assertion below once tracing for the zippering logic is implemented
	assert.Nil(t, logic[cqTask.Id])
}
