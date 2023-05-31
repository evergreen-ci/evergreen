package log

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

// LogLine represents a single line in a log.
type LogLine struct {
	LogName   string
	Timestamp time.Time
	Priority  level.Priority
	Data      string
}

// GetTaskLogs returns the logs from a task run specified by the options.
func GetTaskLogs(ctx context.Context, env evergreen.Environment, taskOpts TaskOptions, opts GetOptions) (LogIterator, error) {
	return nil, errors.New("not implemented")
}

// TaskOptions represents the task-level information required to fetch logs
// from an Evergreen task run.
type TaskOptions struct {
	TaskID    string
	Execution int
}

// GetOptions represents the arguments for fetching logs.
type GetOptions struct {
	// ServiceVersion is the version of the backing logger service.
	ServiceVersion int
	// LogNames are the names of the logs to fetch and merge, prefixes may
	// be specified. At least one name must be specified.
	LogNames []string
	// Start is the start time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Optional.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds.
	End *int64
	// LineLimit specifies the number of log lines to return. Invalid if
	// specified along with Tail or PageSize.
	LineLimit *int
	// Tail specifies the last N log lines to return. Invalid if specified
	// along with LineLimit or PageSize.
	Tail *int
	// PageSize is the number of bytes for byte-based pagination. Invalid
	// if specified along with LineLimit or Tail.
	PageSize *int
}
