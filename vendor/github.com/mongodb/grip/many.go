/*
Multi Logging

The logging methods that end with "Many" allow you to pass a variable
number of message objects for separate logging as a single group, if
your application generates and collects messages asynchronously.
*/
package grip

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

func LogMany(l level.Priority, msgs ...message.Composer) {
	std.LogMany(l, msgs...)
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

func LogManyWhen(conditional bool, l level.Priority, msgs ...message.Composer) {
	std.LogManyWhen(conditional, l, msgs...)
}

func EmergencyManyWhen(conditional bool, msgs ...message.Composer) {
	std.EmergencyManyWhen(conditional, msgs...)
}

func AlertManyWhen(conditional bool, msgs ...message.Composer) {
	std.AlertManyWhen(conditional, msgs...)
}

func CriticalManyWhen(conditional bool, msgs ...message.Composer) {
	std.CriticalManyWhen(conditional, msgs...)
}

func ErrorManyWhen(conditional bool, msgs ...message.Composer) {
	std.ErrorManyWhen(conditional, msgs...)
}

func WarningManyWhen(conditional bool, msgs ...message.Composer) {
	std.WarningManyWhen(conditional, msgs...)
}

func NoticeManyWhen(conditional bool, msgs ...message.Composer) {
	std.NoticeManyWhen(conditional, msgs...)
}

func InfoManyWhen(conditional bool, msgs ...message.Composer) {
	std.InfoManyWhen(conditional, msgs...)
}

func DebugManyWhen(conditional bool, msgs ...message.Composer) {
	std.DebugManyWhen(conditional, msgs...)
}
