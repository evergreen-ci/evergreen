package logging

import "github.com/mongodb/grip/level"

// DefaultLevel returns the current default level for the logger. The
// default level is used for the Default logging methods and as a
// fallback as needed.
func (g *Grip) DefaultLevel() level.Priority {
	return g.Level().Default
}

// SetDefaultLevel configures the logging instance to use the
// specified level. Callers can specify priority as strings, integers,
// or as level.Priority values. If the specified value is not a value,
// uses the current default value.
func (g *Grip) SetDefaultLevel(l interface{}) {
	lv := g.Level()
	lv.Default = convertPriority(l, lv.Default)
	g.CatchError(g.SetLevel(lv))
}

// SetThreshold configures the logging instance to use the
// specified level. Callers can specify priority as strings, integers,
// or as level.Priority values. If the specified value is not a value,
// uses the current threshold value.
func (g *Grip) SetThreshold(l interface{}) {
	lv := g.Level()
	lv.Threshold = convertPriority(l, lv.Threshold)
	g.CatchError(g.SetLevel(lv))
}

// ThresholdLevel returns the current threshold for the logging
// instance. Loggable message above the threshold are always written,
// but messages below the current threshold are not sent or logged.
func (g *Grip) ThresholdLevel() level.Priority {
	return g.Level().Threshold
}

func convertPriority(priority interface{}, fallback level.Priority) level.Priority {
	switch p := priority.(type) {
	case level.Priority:
		return p
	case int:
		return level.Priority(p)
	case string:
		l := level.FromString(p)
		if l == level.Invalid {
			return fallback
		}
		return l
	default:
		return fallback
	}
}
