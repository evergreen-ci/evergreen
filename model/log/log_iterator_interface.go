package log

import "context"

// LogIterator is an interface that enables iterating over lines of Evergreen
// logs.
type LogIterator interface {
	// Next returns true if the iterator has not yet been exhausted or
	// closed, false otherwise.
	Next(context.Context) bool
	// Item returns the current LogLine item held by the iterator.
	Item() LogLine
	// Reverse returns a reversed copy of the iterator.
	Reverse() LogIterator
	// IsReversed returns true if the iterator is in reverse order and
	// false otherwise.
	IsReversed() bool
	// Exhausted returns true if the iterator has not yet been exhausted,
	// regardless if it has been closed or not.
	Exhausted() bool
	// Err returns any errors that are captured by the iterator.
	Err() error
	// Close closes the iterator. This function should be called once the
	// iterator is no longer needed.
	Close() error
}
