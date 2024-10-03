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
