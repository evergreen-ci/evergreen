package log

import (
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// LogIterator is an interface that enables iterating over lines of Evergreen
// logs.
type LogIterator interface {
	// Next returns true if the iterator has not yet been exhausted or
	// closed, false otherwise.
	Next() bool
	// Item returns the current log line held by the iterator.
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

// newTailIterator converts a log iterator into a basic iterator that reads the
// the last N lines of the merged logs.
//
// Only call this function with log iterators that do not support tailing
// natively.
func newTailIterator(it LogIterator, n int) (*basicIterator, error) {
	var items []LogLine
	for it.Next() {
		items = append(items, it.Item())
	}
	catcher := grip.NewBasicCatcher()
	catcher.Add(it.Err())
	catcher.Add(it.Close())

	if len(items) > n {
		items = items[len(items)-n:]
	}

	return newBasicIterator(items), errors.Wrap(catcher.Resolve(), "creating new log tail iterator")
}
