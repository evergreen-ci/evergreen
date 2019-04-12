package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadHostTaskRelationship(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	require.NoError(db.Clear(task.Collection))

	h := &host.Host{}
	var k *task.Task
	assert.False(badHostTaskRelationship(h, k), "false for nil task")

	k = &task.Task{Id: "1"}
	h = &host.Host{RunningTask: "1"}
	assert.False(badHostTaskRelationship(h, k), "false if task id matches running task")

	h.RunningTask = ""
	h.LastTask = "1"
	assert.False(badHostTaskRelationship(h, k), "false if this is last task, and running task is empty")

	h.RunningTask = "2"
	runningTask := &task.Task{Id: "2", Status: evergreen.TaskStarted}
	require.NoError(runningTask.Insert())
	assert.True(badHostTaskRelationship(h, k), "true if task is in a state other than dispatched")

	require.NoError(db.Clear(task.Collection))
	runningTask.Status = evergreen.TaskDispatched
	require.NoError(runningTask.Insert())
	assert.False(badHostTaskRelationship(h, k), "false if task is dispatched")
}
