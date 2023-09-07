package log

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// LogLine represents a single line in an Evergreen log.
type LogLine struct {
	LogName   string
	Priority  level.Priority
	Timestamp int64
	Data      string
}

// GetOptions represents the arguments for fetching Evergreen logs.
type GetOptions struct {
	// Version is the version of the underlying log service with which the
	// specified logs were stored.
	Version int
	// LogNames are the names of the logs to fetch and merge, prefixes may
	// be specified. At least one name must be specified.
	LogNames []string
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

// Get returns the Evergreen logs from the backing service specified by the
// options.
func Get(ctx context.Context, env evergreen.Environment, opts GetOptions) (LogIterator, error) {
	return nil, errors.New("not implemented")
}

// StreamFromLogIterator streams log lines from the given iterator to the
// returned channel. It is the responsibility of the caller to close the
// iterator.
func StreamFromLogIterator(it LogIterator) chan LogLine {
	logLines := make(chan LogLine)
	go func() {
		defer recovery.LogStackTraceAndContinue("streaming lines from log iterator")
		defer close(logLines)

		for it.Next() {
			logLines <- it.Item()
		}

		if err := it.Err(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "streaming lines from log iterator",
			}))
		}
	}()

	return logLines
}
