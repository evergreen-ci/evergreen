package tasklogs

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/taskbuild"
	"github.com/pkg/errors"
)

// TODO: warning not to change this.
const Root = "task_logs"

type TaskLogType string

const (
	TaskLogTypeAll    TaskLogType = "all"
	TaskLogTypeAgent              = "agent"
	TaskLogTypeSystem             = "system"
	TaskLogTypeTask               = "task"
)

func (t TaskLogType) validate() error {
	switch t {
	case TaskLogTypeAll, TaskLogTypeAgent, TaskLogTypeSystem, TaskLogTypeTask:
		return nil
	default:
		return errors.Errorf("unrecognized task log type '%s'", t)
	}
}

// GetOptions represents the arguments for fetching task logs belonging to a
// task run's build.
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

// Get returns the task logs belonging to the specified task run's build.
func Get(ctx context.Context, env evergreen.Environment, taskOpts taskbuild.TaskOptions, getOpts GetOptions) (log.LogIterator, error) {
	if err := getOpts.LogType.validate(); err != nil {
		return nil, err
	}

	return log.Get(ctx, env, log.GetOptions{
		LogNames:  []string{getLogPath(taskOpts, getOpts.LogType)},
		Start:     getOpts.Start,
		End:       getOpts.End,
		LineLimit: getOpts.LineLimit,
		TailN:     getOpts.TailN,
		Version:   getLogServiceVersion(taskOpts.TaskBuildVersion),
	})
}

func getLogPath(taskOpts taskbuild.TaskOptions, logType TaskLogType) string {
	prefix := fmt.Sprintf("%s/%s", taskOpts.Prefix(), Root)

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

func getLogServiceVersion(taskBuildVersion int) int {
	switch {
	case taskBuildVersion >= 0:
		return 0
	default:
		return -1
	}
}
