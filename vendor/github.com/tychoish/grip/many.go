/*
Multi Logging

The logging methods that end with "Many" allow you to pass a variable
number of message objects for separate logging as a single group, if
your application generates and collects messages asynchronously.
*/
package grip

import (
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

func LogMany(l level.Priority, msgs ...message.Composer) {
	std.LogMany(l, msgs...)
}

func DefaultMany(msgs ...message.Composer) {
	std.DefaultMany(msgs...)
}

func EmergencyMany(msgs ...message.Composer) {
	std.EmergencyMany(msgs...)
}

func AlertMany(msgs ...message.Composer) {
	std.AlertMany(msgs...)
}

func CriticalMany(msgs ...message.Composer) {
	std.CriticalMany(msgs...)
}

func ErrorMany(msgs ...message.Composer) {
	std.ErrorMany(msgs...)
}

func WarningMany(msgs ...message.Composer) {
	std.WarningMany(msgs...)
}

func NoticeMany(msgs ...message.Composer) {
	std.NoticeMany(msgs...)
}

func InfoMany(msgs ...message.Composer) {
	std.InfoMany(msgs...)
}

func DebugMany(msgs ...message.Composer) {
	std.DebugMany(msgs...)
}
