package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(task.Collection))

	finished, generateErrs, err := GeneratePoll(ctx, "task-0")
	assert.False(t, finished)
	assert.Empty(t, generateErrs)
	assert.Error(t, err)

	require.NoError(t, (&task.Task{
		Id:             "task-1",
		Version:        "version-1",
		GenerateTask:   true,
		GeneratedTasks: false,
	}).Insert())

	finished, generateErrs, err = GeneratePoll(ctx, "task-1")
	assert.False(t, finished)
	assert.Empty(t, generateErrs)
	assert.NoError(t, err)

	require.NoError(t, (&task.Task{
		Id:             "task-2",
		Version:        "version-1",
		GenerateTask:   true,
		GeneratedTasks: true,
	}).Insert())

	finished, generateErrs, err = GeneratePoll(ctx, "task-2")
	assert.True(t, finished)
	assert.Empty(t, generateErrs)
	assert.NoError(t, err)

	require.NoError(t, (&task.Task{
		Id:                 "task-3",
		Version:            "version-1",
		GenerateTask:       true,
		GeneratedTasks:     true,
		GenerateTasksError: "this is an error",
	}).Insert())

	finished, generateErrs, err = GeneratePoll(ctx, "task-3")
	assert.True(t, finished)
	assert.Equal(t, "this is an error", generateErrs)
	assert.NoError(t, err)
}
