package client

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
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

func TestLoggerProducerRedactorOptions(t *testing.T) {
	secret_key := "secret_key"
	secret_key_redaction := fmt.Sprintf("<REDACTED:%s>", secret_key)
	secret := "super_soccer_ball"
	createTask := func() *task.Task {
		return &task.Task{
			Id:      "task",
			Project: "project",
			TaskOutputInfo: &task.TaskOutput{
				TaskLogs: task.TaskLogOutput{
					Version: 1,
					BucketConfig: evergreen.BucketConfig{
						Name: t.TempDir(),
						Type: evergreen.BucketTypeLocal,
					},
				},
			},
		}
	}

	readLogs := func(t *testing.T, task *task.Task) string {
		logFile := filepath.Join(task.TaskOutputInfo.TaskLogs.BucketConfig.Name, "project", "task", "0", "task_logs", "task")
		entries, err := os.ReadDir(logFile)
		require.NoError(t, err)
		require.Len(t, entries, 1)

		data, err := os.ReadFile(filepath.Join(logFile, entries[0].Name()))
		require.NoError(t, err)

		return string(data)
	}

	t.Run("LeaksWithoutRedactor", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		task := createTask()
		comm := newBaseCommunicator("whatever", map[string]string{})
		logger, err := comm.GetLoggerProducer(ctx, task, nil)
		require.NoError(t, err)

		logger.Task().Alert("Fluff 1")
		logger.Task().Alert("More fluff")
		require.NoError(t, err)

		logger.Task().Info(secret)
		logger.Task().Alert("Even more fluff")
		require.NoError(t, logger.Close())

		data := readLogs(t, task)
		assert.Contains(t, data, secret)
		assert.NotContains(t, data, secret_key_redaction)

		// Make sure it has the other log lines.
		assert.Contains(t, data, "Fluff 1")
		assert.Contains(t, data, "More fluff")
		assert.Contains(t, data, "Even more fluff")
	})

	t.Run("RedactsWhenAdded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		task := createTask()
		comm := newBaseCommunicator("whatever", map[string]string{})
		e := agentutil.NewDynamicExpansions(util.Expansions{})
		logger, err := comm.GetLoggerProducer(ctx, task, &LoggerConfig{
			RedactorOpts: redactor.RedactionOptions{
				Expansions: e,
			},
		})
		logger.Task().Alert("Fluff 1")
		e.PutAndRedact(secret_key, secret)
		logger.Task().Alert("More fluff")
		require.NoError(t, err)

		logger.Task().Info(secret)
		logger.Task().Alert("Even more fluff")
		require.NoError(t, logger.Close())

		data := readLogs(t, task)
		assert.NotContains(t, data, secret)
		assert.Contains(t, data, secret_key_redaction)

		// Make sure it has the other log lines.
		assert.Contains(t, data, "Fluff 1")
		assert.Contains(t, data, "More fluff")
		assert.Contains(t, data, "Even more fluff")
	})
}

func TestLoggerClose(t *testing.T) {
	tsk := &task.Task{
		Id:      "task",
		Project: "project",
		TaskOutputInfo: &task.TaskOutput{
			TaskLogs: task.TaskLogOutput{
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
