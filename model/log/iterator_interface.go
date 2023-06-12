package log

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

type lineParser func(string) (LogLine, error)
