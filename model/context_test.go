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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(task.Collection, task.OldCollection, ProjectRefCollection))

	assert := assert.New(t)
	myProject := ProjectRef{
		Id: "proj",
	}
	assert.NoError(myProject.Insert(ctx))
	newTask := task.Task{
		Id: "newtask",
	}
	oldTask := task.Task{
		Id: "oldtask",
	}
	assert.NoError(newTask.Insert(ctx))
	assert.NoError(oldTask.Insert(ctx))
	assert.NoError(oldTask.Archive(ctx))

	// test that current tasks are loaded correctly
	c, err := LoadContext(ctx, newTask.Id, "", "", "", myProject.Id)
	assert.NoError(err)
	assert.Equal(newTask.Id, c.Task.Id)

	// test that old tasks are loaded correctly
	c, err = LoadContext(ctx, oldTask.Id, "", "", "", myProject.Id)
	assert.NoError(err)
	assert.Equal(oldTask.Id, c.Task.Id)
}
