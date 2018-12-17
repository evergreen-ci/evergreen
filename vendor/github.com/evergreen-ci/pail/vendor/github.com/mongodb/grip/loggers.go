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
func EmergencyPanicf(msg string, a ...interface{}) {
	std.EmergencyPanicf(msg, a...)
}
func EmergencyPanicln(a ...interface{}) {
	std.EmergencyPanicln(a...)
}
func EmergencyFatalf(msg string, a ...interface{}) {
	std.EmergencyFatalf(msg, a...)
}
func EmergencyFatalln(a ...interface{}) {
	std.EmergencyFatalln(a...)
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
