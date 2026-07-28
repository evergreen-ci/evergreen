package task

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetS3LogUsageFromS3(t *testing.T) {
	makeTask := func(t *testing.T) *Task {
		return &Task{
			Id:        "task1",
			Project:   "proj",
			Execution: 0,
			TaskOutputInfo: &TaskOutput{
				TaskLogs: TaskLogOutput{
					Version: 1,
					BucketConfig: evergreen.BucketConfig{
						Type: evergreen.BucketTypeLocal,
						Name: t.TempDir(),
					},
				},
			},
		}
	}

	writeLogChunks := func(t *testing.T, tsk *Task, logType TaskLogType, n int) {
		output, ok := tsk.GetTaskOutputSafe()
		require.True(t, ok)
		bucket, err := newBucket(t.Context(), output.TaskLogs.BucketConfig, nil)
		require.NoError(t, err)
		svc := log.NewLogServiceV0(bucket)
		logName := getLogName(*tsk, logType, output.TaskLogs.ID())
		for i := range n {
			_, _, err = svc.Append(t.Context(), logName, i+1, []log.LogLine{
				{Priority: level.Info, Timestamp: time.Now().UnixNano(), Data: "line"},
			})
			require.NoError(t, err)
		}
	}

	writeTestLogChunks := func(t *testing.T, tsk *Task, logPath string, n int) {
		output, ok := tsk.GetTaskOutputSafe()
		require.True(t, ok)
		bucket, err := newBucket(t.Context(), output.TestLogs.BucketConfig, nil)
		require.NoError(t, err)
		svc := log.NewLogServiceV0(bucket)
		logName := getLogNames(*tsk, []string{logPath}, output.TestLogs.ID())[0]
		for i := range n {
			_, _, err = svc.Append(t.Context(), logName, i+1, []log.LogLine{
				{Priority: level.Info, Timestamp: time.Now().UnixNano(), Data: "line"},
			})
			require.NoError(t, err)
		}
	}

	t.Run("UndispatchedTaskReturnsZeroUsage", func(t *testing.T) {
		tsk := &Task{Id: "no-output", Status: evergreen.TaskUndispatched}
		usage, err := tsk.GetS3LogUsageFromS3(t.Context())
		require.NoError(t, err)
		assert.Zero(t, usage.PutRequests)
		assert.Zero(t, usage.UploadBytes)
	})

	t.Run("BaselineNoChunksUploadedReturnsZeroUsage", func(t *testing.T) {
		tsk := makeTask(t)
		usage, err := tsk.GetS3LogUsageFromS3(t.Context())
		require.NoError(t, err)
		assert.Zero(t, usage.PutRequests)
		assert.Zero(t, usage.UploadBytes)
	})

	t.Run("AbortPathCountsAllLogChunks", func(t *testing.T) {
		tsk := makeTask(t)
		writeLogChunks(t, tsk, TaskLogTypeAgent, 2)
		writeLogChunks(t, tsk, TaskLogTypeTask, 3)

		usage, err := tsk.GetS3LogUsageFromS3(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 5, usage.PutRequests)
		assert.Positive(t, usage.UploadBytes)
		assert.NotEmpty(t, usage.Agent.LogKey)
		assert.Positive(t, usage.Agent.Bytes)
		assert.NotEmpty(t, usage.Task.LogKey)
		assert.Positive(t, usage.Task.Bytes)
	})

	t.Run("SystemFailureLogsCountsAllLogTypes", func(t *testing.T) {
		tsk := makeTask(t)
		writeLogChunks(t, tsk, TaskLogTypeAgent, 1)
		writeLogChunks(t, tsk, TaskLogTypeSystem, 2)
		writeLogChunks(t, tsk, TaskLogTypeTask, 1)

		usage, err := tsk.GetS3LogUsageFromS3(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 4, usage.PutRequests)
		assert.Positive(t, usage.UploadBytes)
		assert.NotEmpty(t, usage.Agent.LogKey)
		assert.Positive(t, usage.Agent.Bytes)
		assert.NotEmpty(t, usage.System.LogKey)
		assert.Positive(t, usage.System.Bytes)
		assert.NotEmpty(t, usage.Task.LogKey)
		assert.Positive(t, usage.Task.Bytes)
	})

	t.Run("TestLogsInSeparateBucketAreCounted", func(t *testing.T) {
		tsk := makeTask(t)
		tsk.TaskOutputInfo.TestLogs = TestLogOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Type: evergreen.BucketTypeLocal,
				Name: t.TempDir(),
			},
		}
		writeLogChunks(t, tsk, TaskLogTypeTask, 2)
		writeTestLogChunks(t, tsk, "TestFoo", 3)

		usage, err := tsk.GetS3LogUsageFromS3(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 5, usage.PutRequests)
		assert.Positive(t, usage.UploadBytes)
		assert.NotEmpty(t, usage.Task.LogKey)
		assert.Positive(t, usage.Task.Bytes)
		assert.NotEmpty(t, usage.Test.LogKey)
		assert.Positive(t, usage.Test.Bytes)
	})

	t.Run("InvalidBucketTypeReturnsError", func(t *testing.T) {
		tsk := &Task{
			Id:        "task-bad-bucket",
			Project:   "proj",
			Execution: 0,
			TaskOutputInfo: &TaskOutput{
				TaskLogs: TaskLogOutput{
					Version: 1,
					BucketConfig: evergreen.BucketConfig{
						Type: "invalid",
						Name: "irrelevant",
					},
				},
			},
		}
		_, err := tsk.GetS3LogUsageFromS3(t.Context())
		require.Error(t, err)
	})
}
