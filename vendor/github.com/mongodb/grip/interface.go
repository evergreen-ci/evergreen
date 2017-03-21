package grip

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

// Journaler describes the public interface of the the Grip
// interface. Used to enforce consistency between the grip and logging
// packages.
type Journaler interface {
	Name() string
	SetName(string)

	// Methods to access the underlying message sending backend.
	GetSender() send.Sender
	SetSender(send.Sender) error

	// Configure the default and threshold levels of the current
	// sender.
	SetThreshold(interface{})
	ThresholdLevel() level.Priority
	SetDefaultLevel(interface{})
	DefaultLevel() level.Priority

	// Specify a log level as an argument rather than a method
	// name.
	Log(level.Priority, interface{})
	Logf(level.Priority, string, ...interface{})
	Logln(level.Priority, ...interface{})
	LogMany(level.Priority, ...message.Composer)
	LogWhen(bool, level.Priority, interface{})
	LogWhenf(bool, level.Priority, string, ...interface{})
	LogWhenln(bool, level.Priority, ...interface{})
	LogManyWhen(bool, level.Priority, ...message.Composer)

	// Log a message (the contents of the error,) only if the
	// error is non-nil. These are redundant to the similar base
	// methods. (e.g. Alert and CatchAlert have the same behavior.)
	CatchLog(level.Priority, error)
	CatchEmergency(error)
	CatchAlert(error)
	CatchCritical(error)
	CatchError(error)
	CatchWarning(error)
	CatchNotice(error)
	CatchInfo(error)
	CatchDebug(error)

	// Methods for sending messages at specific levels. If you
	// send a message at a level that is below the threshold, then it is a no-op.

	// Emergency methods have "panic" and "fatal" variants that
	// call panic or os.Exit(1). It is impossible for "Emergency"
	// to be below threshold, however, if the message isn't
	// loggable (e.g. error is nil, or message is empty,) these
	// methods will not panic/error.
	CatchEmergencyFatal(error)
	CatchEmergencyPanic(error)
	EmergencyFatal(interface{})
	EmergencyFatalf(string, ...interface{})
	EmergencyFatalln(...interface{})
	EmergencyPanic(interface{})
	EmergencyPanicf(string, ...interface{})
	EmergencyPanicln(...interface{})

	// For each level, in addition to a basic logger that takes
	// strings and message.Composer objects (and tries to do its best
	// with everythingelse.) there are println and printf
	// loggers. Each Level also has "When" variants that only log
	// if the passed condition are true.

	Emergency(interface{})
	Emergencyf(string, ...interface{})
	Emergencyln(...interface{})
	EmergencyMany(...message.Composer)
	EmergencyWhen(bool, interface{})
	EmergencyWhenf(bool, string, ...interface{})
	EmergencyWhenln(bool, ...interface{})
	EmergencyManyWhen(bool, ...message.Composer)

	Alert(interface{})
	Alertf(string, ...interface{})
	Alertln(...interface{})
	AlertMany(...message.Composer)
	AlertWhen(bool, interface{})
	AlertWhenf(bool, string, ...interface{})
	AlertWhenln(bool, ...interface{})
	AlertManyWhen(bool, ...message.Composer)

	Critical(interface{})
	Criticalf(string, ...interface{})
	Criticalln(...interface{})
	CriticalMany(...message.Composer)
	CriticalWhen(bool, interface{})
	CriticalWhenf(bool, string, ...interface{})
	CriticalWhenln(bool, ...interface{})
	CriticalManyWhen(bool, ...message.Composer)

	Error(interface{})
	Errorf(string, ...interface{})
	Errorln(...interface{})
	ErrorMany(...message.Composer)
	ErrorWhen(bool, interface{})
	ErrorWhenf(bool, string, ...interface{})
	ErrorWhenln(bool, ...interface{})
	ErrorManyWhen(bool, ...message.Composer)

	Warning(interface{})
	Warningf(string, ...interface{})
	Warningln(...interface{})
	WarningMany(...message.Composer)
	WarningWhen(bool, interface{})
	WarningWhenf(bool, string, ...interface{})
	WarningWhenln(bool, ...interface{})
	WarningManyWhen(bool, ...message.Composer)

	Notice(interface{})
	Noticef(string, ...interface{})
	Noticeln(...interface{})
	NoticeMany(...message.Composer)
	NoticeWhen(bool, interface{})
	NoticeWhenf(bool, string, ...interface{})
	NoticeWhenln(bool, ...interface{})
	NoticeManyWhen(bool, ...message.Composer)

	Info(interface{})
	Infof(string, ...interface{})
	Infoln(...interface{})
	InfoMany(...message.Composer)
	InfoWhen(bool, interface{})
	InfoWhenf(bool, string, ...interface{})
	InfoWhenln(bool, ...interface{})
	InfoManyWhen(bool, ...message.Composer)

	Debug(interface{})
	Debugf(string, ...interface{})
	Debugln(...interface{})
	DebugMany(...message.Composer)
	DebugWhen(bool, interface{})
	DebugWhenf(bool, string, ...interface{})
	DebugWhenln(bool, ...interface{})
	DebugManyWhen(bool, ...message.Composer)
}
