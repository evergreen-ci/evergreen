package testlogs

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
)

// TestLogs is the versioned entry point for coordinating persistent storage
// of a task run's test log data.
type TestLogs int

// ID returns the unique identifier of the test logs output type.
func (TestLogs) ID() string { return "test_logs" }

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

// GetOptions represents the arguments for fetching test logs belonging to a
// task run.
type GetOptions struct {
	// LogPaths are the paths of the logs to fetch and merge, prefixes may
	// be specified. At least one name must be specified.
	LogPaths []string
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

// Get returns the test logs belonging to the specified task run.
func (o TestLogs) Get(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, getOpts GetOptions) (log.LogIterator, error) {
	return log.Get(ctx, env, log.GetOptions{
		LogNames:  o.getLogNames(taskOpts, getOpts.LogPaths),
		Start:     getOpts.Start,
		End:       getOpts.End,
		LineLimit: getOpts.LineLimit,
		TailN:     getOpts.TailN,
		Version:   o.getLogServiceVersion(),
	})
}

func (o TestLogs) getLogNames(taskOpts TaskOptions, logPaths []string) []string {
	prefix := fmt.Sprintf("%s/%s/%d/%s", taskOpts.ProjectID, taskOpts.TaskID, taskOpts.Execution, o.ID())

	logNames := make([]string, len(logPaths))
	for i, path := range logPaths {
		logNames[i] = fmt.Sprintf("%s/%s", prefix, path)
	}

	return logNames
}

func (o TestLogs) getLogServiceVersion() int {
	switch {
	case o >= 0:
		return 0
	default:
		return -1
	}
}
