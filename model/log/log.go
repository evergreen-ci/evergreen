package log

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

// LogLine represents a single line in a log.
type LogLine struct {
	LogName   string
	Priority  level.Priority
	Timestamp int64
	Data      string
}

// GetOptions represents the arguments for fetching logs.
type GetOptions struct {
	// LogNames are the names of the logs to fetch and merge, prefixes may
	// be specified. At least one name must be specified.
	LogNames []string
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	End *int64
}

// TaskOptions represents the task-level information required to fetch logs
// belonging to an Evergreen task run.
type TaskOptions struct {
	TaskID    string
	Execution int
	// ServiceVersion is the version of the backing logger service.
	ServiceVersion int
}

// TaskLogType represents the recognized log types collected during a task run.
type TaskLogType string

const (
	// TaskLogTypeAll includes agent, task, and system logs.
	TaskLogTypeAll    = "all"
	TaskLogTypeAgent  = "agent"
	TaskLogTypeTask   = "task"
	TaskLogTypeSystem = "system"
	TaskLogTypeTest   = "test"
)

// GetTaskLogTypePrefix returns the appropriate "path" prefix for the given log
// type.
func GetTaskLogTypePrefix(env evergreen.Environment, taskOpts TaskOptions, opts GetOptions) (string, error) {
	return "", errors.New("not implemented")
}

// GetTaskLogs returns the logs from a task run specified by the options.
func GetTaskLogs(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, opts GetOptions) (LogIterator, error) {
	return nil, errors.New("not implemented")
}
