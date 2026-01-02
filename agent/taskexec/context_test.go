package taskexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestLogger() grip.Journaler {
	return grip.NewJournaler("test_logger")
}

func setupTestTaskConfig() *TaskConfig {
	opts := LocalTaskConfigOptions{
		TaskName: "test_task",
		WorkDir:  "/test/dir",
	}
	config, _ := NewLocalTaskConfig(opts)
	return config
}

func TestNewTaskContext(t *testing.T) {
	t.Run("ValidInputs", func(t *testing.T) {
		config := setupTestTaskConfig()
		logger := setupTestLogger()

		ctx, err := NewTaskContext(config, logger)
		require.NoError(t, err)
		assert.NotNil(t, ctx)

		assert.Equal(t, "test_task", ctx.GetTaskName())
		assert.Equal(t, "/test/dir", ctx.GetWorkingDirectory())
		assert.NotNil(t, ctx.GetLogger())
		assert.NotNil(t, ctx.GetTaskConfig())
		assert.False(t, ctx.taskStartTime.IsZero())
		assert.False(t, ctx.lastHeartbeat.IsZero())
	})

	t.Run("NilConfig", func(t *testing.T) {
		logger := setupTestLogger()

		ctx, err := NewTaskContext(nil, logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "task config is required")
		assert.Nil(t, ctx)
	})

	t.Run("NilLogger", func(t *testing.T) {
		config := setupTestTaskConfig()

		ctx, err := NewTaskContext(config, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "logger is required")
		assert.Nil(t, ctx)
	})
}

func TestTaskContextTiming(t *testing.T) {
	config := setupTestTaskConfig()
	logger := setupTestLogger()
	ctx, err := NewTaskContext(config, logger)
	require.NoError(t, err)

	t.Run("MarkStart", func(t *testing.T) {
		initialStart := ctx.taskStartTime
		time.Sleep(10 * time.Millisecond)

		ctx.MarkStart()
		assert.True(t, ctx.taskStartTime.After(initialStart))
	})

	t.Run("MarkEnd", func(t *testing.T) {
		assert.True(t, ctx.taskEndTime.IsZero())

		ctx.MarkEnd()
		assert.False(t, ctx.taskEndTime.IsZero())
		assert.True(t, ctx.taskEndTime.After(ctx.taskStartTime))
	})

	t.Run("GetDuration", func(t *testing.T) {
		ctx.MarkStart()
		time.Sleep(50 * time.Millisecond)

		duration := ctx.GetDuration()
		assert.Greater(t, duration, 50*time.Millisecond)

		ctx.MarkEnd()
		finalDuration := ctx.GetDuration()
		assert.Greater(t, finalDuration, 50*time.Millisecond)

		time.Sleep(10 * time.Millisecond)
		sameDuration := ctx.GetDuration()
		assert.Equal(t, finalDuration, sameDuration)
	})
}

func TestTaskContextHeartbeat(t *testing.T) {
	config := setupTestTaskConfig()
	logger := setupTestLogger()
	ctx, err := NewTaskContext(config, logger)
	require.NoError(t, err)

	t.Run("Heartbeat", func(t *testing.T) {
		initialHeartbeat := ctx.GetLastHeartbeat()
		time.Sleep(10 * time.Millisecond)

		ctx.Heartbeat()
		newHeartbeat := ctx.GetLastHeartbeat()
		assert.True(t, newHeartbeat.After(initialHeartbeat))
	})

	t.Run("IsTimedOut", func(t *testing.T) {
		ctx.Heartbeat()

		assert.False(t, ctx.IsTimedOut(1*time.Second))

		ctx.mu.Lock()
		ctx.lastHeartbeat = time.Now().Add(-2 * time.Second)
		ctx.mu.Unlock()

		assert.True(t, ctx.IsTimedOut(1*time.Second))
		assert.False(t, ctx.IsTimedOut(3*time.Second))
	})
}

func TestTaskContextCleanup(t *testing.T) {
	t.Run("NoCleanups", func(t *testing.T) {
		config := setupTestTaskConfig()
		logger := setupTestLogger()
		ctx, err := NewTaskContext(config, logger)
		require.NoError(t, err)

		err = ctx.Cleanup(context.Background())
		assert.NoError(t, err)
		assert.False(t, ctx.taskEndTime.IsZero())
	})

	t.Run("SuccessfulCleanups", func(t *testing.T) {
		config := setupTestTaskConfig()
		logger := setupTestLogger()
		ctx, err := NewTaskContext(config, logger)
		require.NoError(t, err)

		cleanup1Called := false
		cleanup2Called := false

		config.AddCommandCleanup("cmd1", func() error {
			cleanup1Called = true
			return nil
		})

		config.AddCommandCleanup("cmd2", func() error {
			cleanup2Called = true
			return nil
		})

		err = ctx.Cleanup(context.Background())
		assert.NoError(t, err)
		assert.True(t, cleanup1Called)
		assert.True(t, cleanup2Called)
	})

	t.Run("CleanupWithErrors", func(t *testing.T) {
		config := setupTestTaskConfig()
		logger := setupTestLogger()
		ctx, err := NewTaskContext(config, logger)
		require.NoError(t, err)

		config.AddCommandCleanup("failing_cmd", func() error {
			return errors.New("cleanup failed")
		})

		config.AddCommandCleanup("another_failing_cmd", func() error {
			return errors.New("another cleanup failed")
		})

		err = ctx.Cleanup(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cleanup encountered 2 errors")
		assert.Contains(t, err.Error(), "cleanup failed")
	})

	t.Run("MixedCleanupResults", func(t *testing.T) {
		config := setupTestTaskConfig()
		logger := setupTestLogger()
		ctx, err := NewTaskContext(config, logger)
		require.NoError(t, err)

		successCalled := false

		config.AddCommandCleanup("success_cmd", func() error {
			successCalled = true
			return nil
		})

		config.AddCommandCleanup("failing_cmd", func() error {
			return errors.New("this one fails")
		})

		err = ctx.Cleanup(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cleanup encountered 1 errors")
		assert.True(t, successCalled)
	})
}
