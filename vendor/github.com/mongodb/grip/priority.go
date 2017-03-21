package grip

import "github.com/mongodb/grip/level"

// SetDefaultLevel sets the default level for the package level
// Journaler instance.
func SetDefaultLevel(level interface{}) {
	std.SetDefaultLevel(level)
}

// DefaultLevel returns the current default level for the package
// level Journaler instance.
func DefaultLevel() level.Priority {
	return std.DefaultLevel()
}

// SetThreshold sets the threshold level for the package level
// Journaler instance.
func SetThreshold(level interface{}) {
	std.SetThreshold(level)
}

// ThresholdLevel returns the current threshold level for the package
// level Journaler instance.
func ThresholdLevel() level.Priority {
	return std.ThresholdLevel()
}
