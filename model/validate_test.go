package model

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBadHostTaskRelationship(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.Clear(task.Collection))

	var h *host.Host
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

func TestValidateHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostID := "host_id"
	secret := "secret"
	taskID := "task_id"

	for testName, testCase := range map[string]func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header){
		"PassesWithValidHostAndTask": func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header) {
			require.NoError(t, h.Insert(ctx))

			req := &http.Request{Header: header}
			req = req.WithContext(context.WithValue(context.Background(), ApiTaskKey, tsk))

			validatedHost, status, err := ValidateHost(hostID, req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, h, validatedHost)
		},
		"PassesIfHostHasValidTag": func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header) {
			h.Id = ""
			require.NoError(t, h.Insert(ctx))

			req := &http.Request{Header: header}
			req = req.WithContext(context.WithValue(context.Background(), ApiTaskKey, tsk))

			validatedHost, status, err := ValidateHost(hostID, req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, h, validatedHost)
		},
		"FailsWithoutSecret": func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header) {
			require.NoError(t, h.Insert(ctx))

			header.Del(evergreen.HostSecretHeader)
			req := &http.Request{Header: header}
			req = req.WithContext(context.WithValue(context.Background(), ApiTaskKey, tsk))

			validatedHost, status, err := ValidateHost(hostID, req)
			assert.Error(t, err)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Nil(t, validatedHost)
		},
		"FailsWithMismatchedSecret": func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header) {
			require.NoError(t, h.Insert(ctx))

			header.Set(evergreen.HostSecretHeader, "invalid_secret")
			req := &http.Request{Header: header}
			req = req.WithContext(context.WithValue(context.Background(), ApiTaskKey, tsk))

			validatedHost, status, err := ValidateHost(hostID, req)
			assert.Error(t, err)
			assert.Equal(t, http.StatusUnauthorized, status)
			assert.Nil(t, validatedHost)
		},
		"FailsWithoutMatchingID": func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header) {
			require.NoError(t, h.Insert(ctx))

			header.Del(evergreen.HostHeader)
			req := &http.Request{Header: header}
			req = req.WithContext(context.WithValue(context.Background(), ApiTaskKey, tsk))

			validatedHost, status, err := ValidateHost("", req)
			assert.Error(t, err)
			assert.Equal(t, http.StatusBadRequest, status)
			assert.Nil(t, validatedHost)
		},
		"FailsWithoutMatchingTask": func(t *testing.T, h *host.Host, tsk *task.Task, header http.Header) {
			h.RunningTask = "foo"
			require.NoError(t, h.Insert(ctx))

			req := &http.Request{Header: header}
			req = req.WithContext(context.WithValue(context.Background(), ApiTaskKey, tsk))

			validatedHost, status, err := ValidateHost(hostID, req)
			assert.Error(t, err)
			assert.Equal(t, http.StatusConflict, status)
			assert.Nil(t, validatedHost)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(host.Collection))
			}()

			h := &host.Host{
				Id:          hostID,
				Tag:         hostID,
				RunningTask: taskID,
				Secret:      secret,
			}

			tsk := &task.Task{Id: taskID}

			header := http.Header{}
			header.Add(evergreen.HostHeader, hostID)
			header.Add(evergreen.HostSecretHeader, secret)

			testCase(t, h, tsk, header)
		})
	}
}
