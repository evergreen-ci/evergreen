package tasklogs

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/pkg/errors"
)

// TaskLogType represents the recognized types of task logs.
type TaskLogType string

const (
	TaskLogTypeAll    TaskLogType = "all"
	TaskLogTypeAgent  TaskLogType = "agent"
	TaskLogTypeSystem TaskLogType = "system"
	TaskLogTypeTask   TaskLogType = "task"
)

func (t TaskLogType) validate() error {
	switch t {
	case TaskLogTypeAll, TaskLogTypeAgent, TaskLogTypeSystem, TaskLogTypeTask:
		return nil
	default:
		return errors.Errorf("unrecognized task log type '%s'", t)
	}
}

// TaskLogs is the versioned entry point for coordinating persistent storage
// of a task run's task log data.
type TaskLogs int

// ID returns the unique identifier of the task logs output type.
func (TaskLogs) ID() string { return "task_logs" }

// TaskOptions represents the task-level information required for accessing
// task logs belonging to a task run.
type TaskOptions struct {
	// ProjectID is the project ID of the task run.
	ProjectID string
	// TaskID is the task ID of the task run.
	TaskID string
	// Execution is the execution number of the task run.
	Execution int
}

// GetOptions represents the arguments for fetching task logs belonging to a
// task run.
type GetOptions struct {
	// LogType is the type of task log to fetch.
	LogType TaskLogType
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	Start int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	End int64
	// LineLimit limits the number of lines read from the log. Optional.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Optional.
	TailN int
}

// Get returns the task logs belonging to the specified task run.
func (o TaskLogs) Get(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts GetOptions) (log.LogIterator, error) {
	if err := getOpts.LogType.validate(); err != nil {
		return nil, err
	}

	return log.Get(ctx, env, log.GetOptions{
		LogNames:  []string{o.getLogName(taskOpts, getOpts.LogType)},
		Start:     getOpts.Start,
		End:       getOpts.End,
		LineLimit: getOpts.LineLimit,
		TailN:     getOpts.TailN,
		Version:   o.getLogServiceVersion(),
	})
}

func (o TaskLogs) getLogName(taskOpts TaskOptions, logType TaskLogType) string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", taskOpts.ProjectID, taskOpts.TaskID, taskOpts.Execution, o.ID())

	var filename string
	switch logType {
	case TaskLogTypeAgent:
		filename = "agent"
	case TaskLogTypeSystem:
		filename = "system"
	case TaskLogTypeTask:
		filename = "task"
	default:
		return prefix
	}

	return fmt.Sprintf("%s/%s", prefix, filename)
}

func (o TaskLogs) getLogServiceVersion() int {
	switch {
	case o >= 0:
		return 0
	default:
		return -1
	}
}
