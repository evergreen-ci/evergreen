package service

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testutil.Setup()

	require.NoError(t, db.ClearCollections(task.Collection))

	newTask := task.Task{
		HasCedarResults: true,
		Id:              "test_task_id",
		Status:          evergreen.TaskSucceeded,
	}
	err := newTask.Insert()
	require.NoError(t, err)
	uis := UIServer{hostCache: make(map[string]hostCacheItem)}
	projectContext := projectContext{
		Context: model.Context{
			Task: &newTask,
		},
	}
	uiTask := uiTaskData{}
	results := uis.getTestResults(ctx, projectContext, &uiTask)
	assert.Nil(t, results)
}
