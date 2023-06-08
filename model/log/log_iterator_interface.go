package log

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
)

// LogIterator is an interface that enables iterating over lines of Evergreen
// logs.
type LogIterator interface {
	// Next returns true if the iterator has not yet been exhausted or
	// closed, false otherwise.
	Next(context.Context) bool
	// Item returns the current LogLine item held by the iterator.
	Item() LogLine
	// Exhausted returns true if the iterator has not yet been exhausted,
	// regardless if it has been closed or not.
	Exhausted() bool
	// Err returns any errors that are captured by the iterator.
	Err() error
	// Close closes the iterator. This function should be called once the
	// iterator is no longer needed.
	Close() error
}

// StreamFromLogIterator streams log lines from the given log iterator to the
// returned channel. It is the responsibility of the caller to close the
// iterator.
func StreamFromLogIterator(ctx context.Context, iter LogIterator) chan LogLine {
	logLines := make(chan LogLine)
	go func() {
		defer recovery.LogStackTraceAndContinue("streaming lines from log iterator")
		defer close(logLines)

		for iter.Next(ctx) {
			logLines <- iter.Item()
		}

		if err := iter.Err(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "streaming lines from log iterator",
			}))
		}
	}()

	return logLines
}
