package log

import (
	"context"
)

// LogService is a simple abstraction bridging the logical representation of an
// Evergreen log with its physical storage. Namely, it supports writing and
// retrieving logs directly to and from an underlying storage service. Any more
// sophisticated business logic pertaining to log handling (e.g., collection,
// logical organization, retrieval patterns) should be implemented on top of
// this interface in separate layers of the application.
type LogService interface {
	// Get returns a log iterator with the given options.
	Get(context.Context, GetOptions) (LogIterator, error)
	// Append appends given lines to the specified log and sequence chunk.
	// Returns the number of bytes written to storage.
	Append(context.Context, string, int, []LogLine) (int64, error)
}

// GetOptions represents the arguments for fetching Evergreen logs.
type GetOptions struct {
	// LogNames are the names of the logs to fetch and merge, prefixes may
	// be specified. At least one name must be specified.
	//
	// Log lines from multiple logs are always merged in a deterministic
	// order by timestamp.
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

// LineParser functions parse a raw log line into the service representation of
// a log line for uniform ingestion of logs.
// Parsers need not set the log name or, in most cases, the priority.
type LineParser func(string) (LogLine, error)
