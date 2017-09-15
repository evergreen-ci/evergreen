package logging

import "github.com/mongodb/grip/level"

// DefaultLevel returns the current default level for the logger. The
// default level is used for the Default logging methods and as a
// fallback as needed.
func (g *Grip) DefaultLevel() level.Priority {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.impl.Level().Default
}

// SetDefaultLevel configures the logging instance to use the
// specified level. Callers can specify priority as strings, integers,
// or as level.Priority values. If the specified value is not a value,
// uses the current default value.
func (g *Grip) SetDefaultLevel(l interface{}) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	lv := g.impl.Level()
	lv.Default = convertPriority(l, lv.Default)
	return g.impl.SetLevel(lv)

}

// SetThreshold configures the logging instance to use the
// specified level. Callers can specify priority as strings, integers,
// or as level.Priority values. If the specified value is not a value,
// uses the current threshold value.
func (g *Grip) SetThreshold(l interface{}) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	lv := g.impl.Level()
	lv.Threshold = convertPriority(l, lv.Threshold)
	return g.impl.SetLevel(lv)
}

// ThresholdLevel returns the current threshold for the logging
// instance. Loggable message above the threshold are always written,
// but messages below the current threshold are not sent or logged.
func (g *Grip) ThresholdLevel() level.Priority {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.impl.Level().Threshold
}

func convertPriority(priority interface{}, fallback level.Priority) level.Priority {
	switch p := priority.(type) {
	case level.Priority:
		return p
	case int:
		out := level.Priority(p)
		if !level.IsValidPriority(out) {
			return fallback
		}
		return out
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
