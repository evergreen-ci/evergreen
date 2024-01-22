package client

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/stretchr/testify/assert"
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
	tsk := &task.Task{
		Id:      "task",
		Project: "project",
		TaskOutputInfo: &taskoutput.TaskOutput{
			TaskLogs: taskoutput.TaskLogOutput{
				Version: 1,
				BucketConfig: evergreen.BucketConfig{
					Name: t.TempDir(),
					Type: evergreen.BucketTypeLocal,
				},
			},
		},
	}

	comm := NewHostCommunicator("host", "host", "host_secret")
	logger, err := comm.GetLoggerProducer(context.Background(), tsk, nil)
	assert.NoError(t, err)
	assert.NotNil(t, logger)
	assert.NoError(t, logger.Close())
	assert.NotPanics(t, func() {
		assert.NoError(t, logger.Close())
	})
}
