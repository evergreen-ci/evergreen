package log

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
)

// LogLine represents a single line in an Evergreen log.
type LogLine struct {
	LogName   string
	Priority  level.Priority
	Timestamp int64
	Data      string
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
