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
