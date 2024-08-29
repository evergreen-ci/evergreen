package log

import (
	"github.com/mongodb/grip/level"
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
