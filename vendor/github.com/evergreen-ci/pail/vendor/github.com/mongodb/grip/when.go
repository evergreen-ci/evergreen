/*
Conditional Logging

The Conditional logging methods take two arguments, a Boolean, and a
message argument. Messages can be strings, objects that implement the
MessageComposer interface, or errors. If condition boolean is true,
the threshold level is met, and the message to log is not an empty
string, then it logs the resolved message.

Use conditional logging methods to potentially suppress log messages
based on situations orthogonal to log level, with "log sometimes" or
"log rarely" semantics. Combine with MessageComposers to to avoid
expensive message building operations.
*/
package grip

import "github.com/mongodb/grip/level"

func LogWhen(conditional bool, l level.Priority, m interface{}) {
	std.LogWhen(conditional, l, m)
}
func LogWhenln(conditional bool, l level.Priority, msg ...interface{}) {
	std.LogWhenln(conditional, l, msg...)
}
func LogWhenf(conditional bool, l level.Priority, msg string, args ...interface{}) {
	std.LogWhenf(conditional, l, msg, args...)
}

// Emergency-level Conditional Methods

func EmergencyWhen(conditional bool, m interface{}) {
	std.EmergencyWhen(conditional, m)
}
func EmergencyWhenln(conditional bool, msg ...interface{}) {
	std.EmergencyWhenln(conditional, msg...)
}
func EmergencyWhenf(conditional bool, msg string, args ...interface{}) {
	std.EmergencyWhenf(conditional, msg, args...)
}

// Alert-Level Conditional Methods

func AlertWhen(conditional bool, m interface{}) {
	std.AlertWhen(conditional, m)
}
func AlertWhenln(conditional bool, msg ...interface{}) {
	std.AlertWhenln(conditional, msg...)
}
func AlertWhenf(conditional bool, msg string, args ...interface{}) {
	std.AlertWhenf(conditional, msg, args...)
}

// Critical-level Conditional Methods

func CriticalWhen(conditional bool, m interface{}) {
	std.CriticalWhen(conditional, m)
}
func CriticalWhenln(conditional bool, msg ...interface{}) {
	std.CriticalWhenln(conditional, msg...)
}
func CriticalWhenf(conditional bool, msg string, args ...interface{}) {
	std.CriticalWhenf(conditional, msg, args...)
}

// Error-level Conditional Methods

func ErrorWhen(conditional bool, m interface{}) {
	std.ErrorWhen(conditional, m)
}
func ErrorWhenln(conditional bool, msg ...interface{}) {
	std.ErrorWhenln(conditional, msg...)
}
func ErrorWhenf(conditional bool, msg string, args ...interface{}) {
	std.ErrorWhenf(conditional, msg, args...)
}

// Warning-level Conditional Methods

func WarningWhen(conditional bool, m interface{}) {
	std.WarningWhen(conditional, m)
}
func WarningWhenln(conditional bool, msg ...interface{}) {
	std.WarningWhenln(conditional, msg...)
}
func WarningWhenf(conditional bool, msg string, args ...interface{}) {
	std.WarningWhenf(conditional, msg, args...)
}

// Notice-level Conditional Methods

func NoticeWhen(conditional bool, m interface{}) {
	std.NoticeWhen(conditional, m)
}
func NoticeWhenln(conditional bool, msg ...interface{}) {
	std.NoticeWhenln(conditional, msg...)
}
func NoticeWhenf(conditional bool, msg string, args ...interface{}) {
	std.NoticeWhenf(conditional, msg, args...)
}

// Info-level Conditional Methods

func InfoWhen(conditional bool, message interface{}) {
	std.InfoWhen(conditional, message)
}
func InfoWhenln(conditional bool, msg ...interface{}) {
	std.InfoWhenln(conditional, msg...)
}
func InfoWhenf(conditional bool, msg string, args ...interface{}) {
	std.InfoWhenf(conditional, msg, args...)
}

// Debug-level conditional Methods

func DebugWhen(conditional bool, m interface{}) {
	std.DebugWhen(conditional, m)
}
func DebugWhenln(conditional bool, msg ...interface{}) {
	std.DebugWhenln(conditional, msg...)
}
func DebugWhenf(conditional bool, msg string, args ...interface{}) {
	std.DebugWhenf(conditional, msg, args...)
}
