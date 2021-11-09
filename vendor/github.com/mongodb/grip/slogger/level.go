package slogger

import "github.com/mongodb/grip/level"

// Level represents slogger's level types. In the original
// implementation there are four levels and an "OFF" value.
type Level uint8

// slogger has its own system of priorities/log levels. These
// constants represent those levels, and the Level type can be
// converted to grip level.Priority values.
const (
	OFF Level = iota
	DEBUG
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case OFF:
		return "off"
	case DEBUG:
		return "debug"
	case INFO:
		return "info"
	case WARN:
		return "warn"
	case ERROR:
		return "error"
	default:
		return ""
	}
}

// Priority returns grip's native level.Priority for a slogger.Level.
func (l Level) Priority() level.Priority {
	switch l {
	case OFF:
		return level.Invalid
	case DEBUG:
		return level.Debug
	case INFO:
		return level.Info
	case WARN:
		return level.Warning
	case ERROR:
		return level.Error
	default:
		return level.Notice
	}
}

func convertFromPriority(l level.Priority) Level {
	switch l {
	case level.Emergency, level.Alert, level.Critical, level.Error:
		return ERROR
	case level.Warning:
		return WARN
	case level.Notice, level.Info:
		return INFO
	case level.Debug:
		return DEBUG
	case level.Invalid:
		return OFF
	default:
		return INFO
	}
}
