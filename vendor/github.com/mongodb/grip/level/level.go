/*
Package level defines a Priority type and some conversion methods for a 7-tiered
logging level schema, which mirror syslog and system's logging levels.

Levels range from Emergency (0) to Debug (7), and the special type
Priority and associated constants provide access to these values.

*/
package level

import "strings"

// Priority is an integer that tracks log levels. Use with one of the
// defined constants.
type Priority int16

// Constants defined for easy access to
const (
	Emergency Priority = 100
	Alert     Priority = 90
	Critical  Priority = 80
	Error     Priority = 70
	Warning   Priority = 60
	Notice    Priority = 50
	Info      Priority = 40
	Debug     Priority = 30
	Trace     Priority = 20
	Invalid   Priority = 0
)

// String implements the Stringer interface and makes it possible to
// print human-readable string identifier for a log level.
func (p Priority) String() string {
	switch p {
	case 100:
		return "emergency"
	case 90:
		return "alert"
	case 80:
		return "critical"
	case 70:
		return "error"
	case 60:
		return "warning"
	case 50:
		return "notice"
	case 40:
		return "info"
	case 30:
		return "debug"
	case 20:
		return "trace"
	default:
		return "invalid"
	}
}

// IsValid returns false when the priority valid is not a valid
// priority value.
func (p Priority) IsValid() bool {
	return p > 1 && p <= 100
}

// FromString takes a string, (case insensitive, leading and trailing space removed, )
func FromString(l string) Priority {
	switch strings.TrimSpace(strings.ToLower(l)) {
	case "emergency":
		return Emergency
	case "alert":
		return Alert
	case "critical":
		return Critical
	case "error":
		return Error
	case "warning":
		return Warning
	case "notice":
		return Notice
	case "info":
		return Info
	case "debug":
		return Debug
	case "trace":
		return Trace
	default:
		return Invalid
	}
}
