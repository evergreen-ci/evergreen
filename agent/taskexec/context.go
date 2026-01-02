package taskexec

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Command interface {
	Execute(ctx context.Context, config *TaskConfig, logger grip.Journaler) error
	FullDisplayName() string
	Type() string
}

type TaskContext struct {
	taskName      string
	workDir       string
	logger        grip.Journaler
	taskConfig    *TaskConfig
	taskStartTime time.Time
	taskEndTime   time.Time
	lastHeartbeat time.Time
}

func NewTaskContext(config *TaskConfig, logger grip.Journaler) (*TaskContext, error) {
	if config == nil {
		return nil, errors.New("task config is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}

	return &TaskContext{
		taskName:      config.GetTaskName(),
		workDir:       config.WorkDir,
		logger:        logger,
		taskConfig:    config,
		taskStartTime: time.Now(),
		lastHeartbeat: time.Now(),
	}, nil
}

func (tc *TaskContext) GetTaskConfig() *TaskConfig {
	return tc.taskConfig
}

func (tc *TaskContext) GetLogger() grip.Journaler {
	return tc.logger
}

func (tc *TaskContext) GetWorkingDirectory() string {
	return tc.workDir
}

func (tc *TaskContext) GetTaskName() string {
	return tc.taskName
}

func (tc *TaskContext) MarkStart() {
	tc.taskStartTime = time.Now()
}

func (tc *TaskContext) MarkEnd() {
	tc.taskEndTime = time.Now()
}

func (tc *TaskContext) GetDuration() time.Duration {
	if tc.taskEndTime.IsZero() {
		return time.Since(tc.taskStartTime)
	}
	return tc.taskEndTime.Sub(tc.taskStartTime)
}

func (tc *TaskContext) Heartbeat() {
	tc.lastHeartbeat = time.Now()
}

func (tc *TaskContext) GetLastHeartbeat() time.Time {
	return tc.lastHeartbeat
}

func (tc *TaskContext) Cleanup(ctx context.Context) error {
	tc.logger.Info("Cleaning up task context")
	tc.MarkEnd()
	return nil
}
