// Catch Logging
//
// Logging helpers for catching and logging error messages. Helpers exist
// for the following levels, with helpers defined both globally for the
// global logger and for Journaler logging objects.
package grip

import "github.com/mongodb/grip/level"

// Emergency + (fatal/panic)
// Alert + (fatal/panic)
// Critical + (fatal/panic)
// Error + (fatal/panic)
// Warning
// Notice
// Info
// Debug

func CatchLog(l level.Priority, err error) {
	std.CatchLog(l, err)
}

// Level Emergency Catcher Logging Helpers

func CatchEmergency(err error) {
	std.CatchEmergency(err)
}
func CatchEmergencyPanic(err error) {
	std.CatchEmergency(err)
}
func CatchEmergencyFatal(err error) {
	std.CatchEmergencyFatal(err)
}

// Level Alert Catcher Logging Helpers

func CatchAlert(err error) {
	std.CatchAlert(err)
}

// Level Critical Catcher Logging Helpers

func CatchCritical(err error) {
	std.CatchCritical(err)
}

// Level Error Catcher Logging Helpers

func CatchError(err error) {
	std.CatchError(err)
}

// Level Warning Catcher Logging Helpers

func CatchWarning(err error) {
	std.CatchWarning(err)
}

// Level Notice Catcher Logging Helpers

func CatchNotice(err error) {
	std.CatchNotice(err)
}

// Level Info Catcher Logging Helpers

func CatchInfo(err error) {
	std.CatchInfo(err)
}

// Level Debug Catcher Logging Helpers

func CatchDebug(err error) {
	std.CatchDebug(err)
}
