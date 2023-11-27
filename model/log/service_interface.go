package log

import (
	"context"
)

// LogService is the interface for Evergreen log services.
type LogService interface {
	Get(context.Context, GetOptions) (LogIterator, error)
	Append(context.Context, string, []LogLine) error
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
	// represented as a Unix timestamp in nanoseconds. Defaults to
	// unbounded unless DefaultTimeRangeOfFirstLog is set to true.
	Start *int64
	// End is the end time (inclusive) of the time range filter,
	// represented as a Unix timestamp in nanoseconds. Defaults to
	// unbounded unless DefaultTimeRangeOfFirstLog is set to true.
	End *int64
	// DefaultTimeRangeOfFirstLog defaults the start and end time of the
	// time range filter to the first and last timestamp, respectively, of
	// the first specified log in LogNames.
	DefaultTimeRangeOfFirstLog bool
	// LineLimit limits the number of lines read from the log. Ignored if
	// less than or equal to 0.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	// Ignored if less than or equal to 0.
	TailN int
}
