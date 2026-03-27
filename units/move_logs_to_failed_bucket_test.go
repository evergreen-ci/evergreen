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
		job := NewMoveLogsToFailedBucketJob(env, "task-123", "2025-01-16", sourceCfg, MoveLogsTriggerTaskEnd)
		require.NotNil(t, job)

		moveJob, ok := job.(*moveLogsToFailedBucketJob)
		require.True(t, ok)
		assert.Equal(t, "task-123", moveJob.TaskID)
		assert.Equal(t, MoveLogsTriggerTaskEnd, moveJob.Trigger)
		assert.Equal(t, sourceCfg, moveJob.SourceBucketCfg)
		assert.Zero(t, moveJob.Timeout)
		assert.Equal(t, fetchTimeout, job.TimeInfo().MaxTime)
	})

	t.Run("CustomTimeout", func(t *testing.T) {
		customTimeout := 60 * time.Minute
		job := NewMoveLogsToFailedBucketJob(env, "task-456", "2025-01-16", sourceCfg, MoveLogsTriggerTaskEnd, customTimeout)
		require.NotNil(t, job)

		moveJob, ok := job.(*moveLogsToFailedBucketJob)
		require.True(t, ok)
		assert.Equal(t, "task-456", moveJob.TaskID)
		assert.Equal(t, customTimeout, moveJob.Timeout)
		assert.Equal(t, customTimeout, job.TimeInfo().MaxTime)
	})
}
