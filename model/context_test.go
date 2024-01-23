package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadContext(t *testing.T) {
	backgroundCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, ProjectRefCollection))

	assert := assert.New(t)
	myProject := ProjectRef{
		Id: "proj",
	}
	assert.NoError(myProject.Insert())
	newTask := task.Task{
		Id: "newtask",
	}
	oldTask := task.Task{
		Id: "oldtask",
	}
	assert.NoError(newTask.Insert())
	assert.NoError(oldTask.Insert())
	assert.NoError(oldTask.Archive(backgroundCtx))

	// test that current tasks are loaded correctly
	ctx, err := LoadContext(newTask.Id, "", "", "", myProject.Id)
	assert.NoError(err)
	assert.Equal(newTask.Id, ctx.Task.Id)

	// test that old tasks are loaded correctly
	ctx, err = LoadContext(oldTask.Id, "", "", "", myProject.Id)
	assert.NoError(err)
	assert.Equal(oldTask.Id, ctx.Task.Id)
}
