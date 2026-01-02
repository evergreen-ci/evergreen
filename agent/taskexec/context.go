package taskexec

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Command interface {
	command.Command
	// LocalExecute provides local task execution
	// that doesn't require full agent infrastructure.
	LocalExecute(ctx context.Context, config *TaskConfig, logger grip.Journaler) error
}

type TaskContext struct {
	taskName      string
	workDir       string
	logger        grip.Journaler
	taskConfig    *TaskConfig
	taskStartTime time.Time
	taskEndTime   time.Time
	lastHeartbeat time.Time
	mu            sync.RWMutex
}

func NewTaskContext(config *TaskConfig, logger grip.Journaler) (*TaskContext, error) {
	if config == nil {
		return nil, errors.New("task config is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}

	now := time.Now()
	return &TaskContext{
		taskName:      config.GetTaskName(),
		workDir:       config.WorkDir,
		logger:        logger,
		taskConfig:    config,
		taskStartTime: now,
		lastHeartbeat: now,
	}, nil
}

// GetTaskConfig returns the task configuration.
func (tc *TaskContext) GetTaskConfig() *TaskConfig {
	return tc.taskConfig
}

// GetLogger returns the task logger.
func (tc *TaskContext) GetLogger() grip.Journaler {
	return tc.logger
}

// GetWorkingDirectory returns the task's working directory.
func (tc *TaskContext) GetWorkingDirectory() string {
	return tc.workDir
}

// GetTaskName returns the task name.
func (tc *TaskContext) GetTaskName() string {
	return tc.taskName
}

// MarkStart marks the task start time.
func (tc *TaskContext) MarkStart() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.taskStartTime = time.Now()
}

// MarkEnd marks the task end time.
func (tc *TaskContext) MarkEnd() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.taskEndTime = time.Now()
}

// GetDuration returns the task execution duration.
func (tc *TaskContext) GetDuration() time.Duration {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.taskEndTime.IsZero() {
		return time.Since(tc.taskStartTime)
	}
	return tc.taskEndTime.Sub(tc.taskStartTime)
}

// Heartbeat updates the last heartbeat time.
func (tc *TaskContext) Heartbeat() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.lastHeartbeat = time.Now()
}

// GetLastHeartbeat returns the last heartbeat time.
func (tc *TaskContext) GetLastHeartbeat() time.Time {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.lastHeartbeat
}

// IsTimedOut checks if the task has exceeded its timeout based on last heartbeat.
func (tc *TaskContext) IsTimedOut(timeout time.Duration) bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return time.Since(tc.lastHeartbeat) > timeout
}

// Cleanup runs any registered cleanup functions and marks the task as complete.
func (tc *TaskContext) Cleanup(ctx context.Context) error {
	tc.logger.Info("Starting task context cleanup")
	tc.MarkEnd()

	cleanups := tc.taskConfig.GetAndClearCommandCleanups()
	var cleanupErrors []error

	for _, cleanup := range cleanups {
		tc.logger.Infof("Running cleanup for command: %s", cleanup.Command)
		if err := cleanup.Run(); err != nil {
			tc.logger.Errorf("Cleanup failed for command %s: %v", cleanup.Command, err)
			cleanupErrors = append(cleanupErrors, errors.Wrapf(err, "cleanup failed for command %s", cleanup.Command))
		}
	}

	tc.logger.Infof("Task context cleanup completed. Duration: %v", tc.GetDuration())

	if len(cleanupErrors) > 0 {
		return errors.Errorf("cleanup encountered %d errors: %v", len(cleanupErrors), cleanupErrors)
	}

	return nil
}
