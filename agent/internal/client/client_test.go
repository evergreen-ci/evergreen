package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvergreenCommunicatorConstructor(t *testing.T) {
	client := NewHostCommunicator("url", "hostID", "hostSecret")
	defer client.Close()

	c, ok := client.(*hostCommunicator)
	assert.True(t, ok, true)
	assert.Equal(t, "hostID", c.reqHeaders[evergreen.HostHeader])
	assert.Equal(t, "hostSecret", c.reqHeaders[evergreen.HostSecretHeader])
	assert.Equal(t, defaultMaxAttempts, c.retry.MaxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.retry.MinDelay)
	assert.Equal(t, defaultTimeoutMax, c.retry.MaxDelay)
}

func TestLoggerClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)

	server, _ := newMockServer(func(w http.ResponseWriter, _ *http.Request) {
		data, err := json.Marshal(&task.Task{
			Id:      "task",
			Project: "project",
			TaskOutputInfo: &taskoutput.TaskOutput{
				TaskLogs: taskoutput.TaskLogOutput{
					Version:      1,
					BucketConfig: env.Settings().Buckets.LogBucket,
				},
			},
		})
		require.NoError(t, err)

		_, err = w.Write(data)
		require.NoError(t, err)
	})

	comm := NewHostCommunicator(server.URL, "host", "host_secret")
	logger, err := comm.GetLoggerProducer(context.Background(), TaskData{ID: "task", Secret: "task_secret"}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.NoError(t, logger.Close())
	assert.NotPanics(t, func() {
		assert.NoError(t, logger.Close())
	})
}
