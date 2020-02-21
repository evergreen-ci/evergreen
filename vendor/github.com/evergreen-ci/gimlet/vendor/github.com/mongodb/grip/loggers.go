/*
Basic Logging

Loging helpers exist for the following levels:

   Emergency + (fatal/panic)
   Alert + (fatal/panic)
   Critical + (fatal/panic)
   Error + (fatal/panic)
   Warning
   Notice
   Info
   Debug

These methods accept both strings (message content,) or types that
implement the message.MessageComposer interface. Composer types make
it possible to delay generating a message unless the logger is over
the logging threshold. Use this to avoid expensive serialization
operations for suppressed logging operations.

All levels also have additional methods with `ln` and `f` appended to
the end of the method name which allow Println() and Printf() style
functionality. You must pass printf/println-style arguments to these methods.

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

func Log(l level.Priority, msg interface{}) {
	std.Log(l, msg)
}
func Logf(l level.Priority, msg string, a ...interface{}) {
	std.Logf(l, msg, a...)
}
func Logln(l level.Priority, a ...interface{}) {
	std.Logln(l, a...)
}
func LogWhen(conditional bool, l level.Priority, m interface{}) {
	std.LogWhen(conditional, l, m)
}

// Leveled Logging Methods
// Emergency-level logging methods

func EmergencyFatal(msg interface{}) {
	std.EmergencyFatal(msg)
}
func Emergency(msg interface{}) {
	std.Emergency(msg)
}
func Emergencyf(msg string, a ...interface{}) {
	std.Emergencyf(msg, a...)
}
func Emergencyln(a ...interface{}) {
	std.Emergencyln(a...)
}
func EmergencyPanic(msg interface{}) {
	std.EmergencyPanic(msg)
}
func EmergencyWhen(conditional bool, m interface{}) {
	std.EmergencyWhen(conditional, m)
}

// Alert-level logging methods

func Alert(msg interface{}) {
	std.Alert(msg)
}
func Alertf(msg string, a ...interface{}) {
	std.Alertf(msg, a...)
}
func Alertln(a ...interface{}) {
	std.Alertln(a...)
}
func AlertWhen(conditional bool, m interface{}) {
	std.AlertWhen(conditional, m)
}

// Critical-level logging methods

func Critical(msg interface{}) {
	std.Critical(msg)
}
func Criticalf(msg string, a ...interface{}) {
	std.Criticalf(msg, a...)
}
func Criticalln(a ...interface{}) {
	std.Criticalln(a...)
}
func CriticalWhen(conditional bool, m interface{}) {
	std.CriticalWhen(conditional, m)
}

// Error-level logging methods

func Error(msg interface{}) {
	std.Error(msg)
}
func Errorf(msg string, a ...interface{}) {
	std.Errorf(msg, a...)
}
func Errorln(a ...interface{}) {
	std.Errorln(a...)
}
func ErrorWhen(conditional bool, m interface{}) {
	std.ErrorWhen(conditional, m)
}

// Warning-level logging methods

func Warning(msg interface{}) {
	std.Warning(msg)
}
func Warningf(msg string, a ...interface{}) {
	std.Warningf(msg, a...)
}
func Warningln(a ...interface{}) {
	std.Warningln(a...)
}
func WarningWhen(conditional bool, m interface{}) {
	std.WarningWhen(conditional, m)
}

// Notice-level logging methods

func Notice(msg interface{}) {
	std.Notice(msg)
}
func Noticef(msg string, a ...interface{}) {
	std.Noticef(msg, a...)
}
func Noticeln(a ...interface{}) {
	std.Noticeln(a...)
}
func NoticeWhen(conditional bool, m interface{}) {
	std.NoticeWhen(conditional, m)
}

// Info-level logging methods

func Info(msg interface{}) {
	std.Info(msg)
}
func Infof(msg string, a ...interface{}) {
	std.Infof(msg, a...)
}
func Infoln(a ...interface{}) {
	std.Infoln(a...)
}
func InfoWhen(conditional bool, message interface{}) {
	std.InfoWhen(conditional, message)
}

// Debug-level logging methods

func Debug(msg interface{}) {
	std.Debug(msg)
}
func Debugf(msg string, a ...interface{}) {
	std.Debugf(msg, a...)
}
func Debugln(a ...interface{}) {
	std.Debugln(a...)
}
func DebugWhen(conditional bool, m interface{}) {
	std.DebugWhen(conditional, m)
}
