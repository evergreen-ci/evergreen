package grip

import "github.com/mongodb/grip/logging"

// The base type for all Journaling methods provided by the Grip
// package. The package logger uses systemd logging on Linux, when
// possible, falling back to standard output-native when systemd
// logging is not available.

// NewJournaler creates a new Journaler instance. The Sender method is a
// non-operational bootstrap method that stores default and threshold
// types, as needed. You must use SetSender() or the
// UseSystemdLogger(), UseNativeLogger(), or UseFileLogger() methods
// to configure the backend.
func NewJournaler(name string) Journaler {
	return logging.NewGrip(name)
}

// Name of the logger instance
func Name() string {
	return std.Name()
}

// SetName declare a name string for the logger, including in the logging
// message. Typically this is included on the output of the command.
func SetName(name string) {
	std.SetName(name)
}
