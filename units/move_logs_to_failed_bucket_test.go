package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMoveLogsToFailedBucketJob(t *testing.T) {
	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))

	sourceCfg := evergreen.BucketConfig{Name: "test-source-bucket", Type: "s3"}

	t.Run("DefaultTimeout", func(t *testing.T) {
		job := NewMoveLogsToFailedBucketJob(env, "task-123", "2025-01-16", sourceCfg, MoveLogsTriggerTaskEnd, false)
		require.NotNil(t, job)

		moveJob, ok := job.(*moveLogsToFailedBucketJob)
		require.True(t, ok)
		assert.Equal(t, "task-123", moveJob.TaskID)
		assert.Equal(t, MoveLogsTriggerTaskEnd, moveJob.Trigger)
		assert.Equal(t, sourceCfg, moveJob.SourceBucketCfg)
		assert.False(t, moveJob.RunInOldTaskCollection)
		assert.Zero(t, moveJob.Timeout)
		assert.Equal(t, fetchTimeout, job.TimeInfo().MaxTime)
	})

	t.Run("RunInOldTaskCollection", func(t *testing.T) {
		job := NewMoveLogsToFailedBucketJob(env, "task-old-789", "2025-01-16", sourceCfg, MoveLogsTriggerHourlyRetry, true)
		require.NotNil(t, job)

		moveJob, ok := job.(*moveLogsToFailedBucketJob)
		require.True(t, ok)
		assert.Equal(t, "task-old-789", moveJob.TaskID)
		assert.Equal(t, MoveLogsTriggerHourlyRetry, moveJob.Trigger)
		assert.Equal(t, sourceCfg, moveJob.SourceBucketCfg)
		assert.True(t, moveJob.RunInOldTaskCollection)
		assert.Zero(t, moveJob.Timeout)
		assert.Equal(t, fetchTimeout, job.TimeInfo().MaxTime)
	})

	t.Run("CustomTimeout", func(t *testing.T) {
		customTimeout := 60 * time.Minute
		job := NewMoveLogsToFailedBucketJob(env, "task-456", "2025-01-16", sourceCfg, MoveLogsTriggerTaskEnd, false, customTimeout)
		require.NotNil(t, job)

		moveJob, ok := job.(*moveLogsToFailedBucketJob)
		require.True(t, ok)
		assert.Equal(t, "task-456", moveJob.TaskID)
		assert.False(t, moveJob.RunInOldTaskCollection)
		assert.Equal(t, customTimeout, moveJob.Timeout)
		assert.Equal(t, customTimeout, job.TimeInfo().MaxTime)
	})

	t.Run("CustomTimeoutRunInOldTaskCollection", func(t *testing.T) {
		customTimeout := 60 * time.Minute
		job := NewMoveLogsToFailedBucketJob(env, "task-old-999", "2025-01-16", sourceCfg, MoveLogsTriggerHourlyRetry, true, customTimeout)
		require.NotNil(t, job)

		moveJob, ok := job.(*moveLogsToFailedBucketJob)
		require.True(t, ok)
		assert.Equal(t, "task-old-999", moveJob.TaskID)
		assert.Equal(t, MoveLogsTriggerHourlyRetry, moveJob.Trigger)
		assert.True(t, moveJob.RunInOldTaskCollection)
		assert.Equal(t, customTimeout, moveJob.Timeout)
		assert.Equal(t, customTimeout, job.TimeInfo().MaxTime)
	})
}
