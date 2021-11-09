package grip

import (
	"github.com/mongodb/grip/level"
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
	SetLevel(send.LevelInfo) error

	// Send allows you to push a composer which stores its own
	// priorty (or uses the sender's default priority).
	Send(interface{})

	// Specify a log level as an argument rather than a method
	// name.
	Log(level.Priority, interface{})
	Logf(level.Priority, string, ...interface{})
	Logln(level.Priority, ...interface{})
	LogWhen(bool, level.Priority, interface{})

	// Methods for sending messages at specific levels. If you
	// send a message at a level that is below the threshold, then it is a no-op.

	// Emergency methods have "panic" and "fatal" variants that
	// call panic or os.Exit(1). It is impossible for "Emergency"
	// to be below threshold, however, if the message isn't
	// loggable (e.g. error is nil, or message is empty,) these
	// methods will not panic/error.
	EmergencyFatal(interface{})
	EmergencyPanic(interface{})

	// For each level, in addition to a basic logger that takes
	// strings and message.Composer objects (and tries to do its best
	// with everythingelse.) there are println and printf
	// loggers. Each Level also has "When" variants that only log
	// if the passed condition are true.
	Emergency(interface{})
	Emergencyf(string, ...interface{})
	Emergencyln(...interface{})
	EmergencyWhen(bool, interface{})

	Alert(interface{})
	Alertf(string, ...interface{})
	Alertln(...interface{})
	AlertWhen(bool, interface{})

	Critical(interface{})
	Criticalf(string, ...interface{})
	Criticalln(...interface{})
	CriticalWhen(bool, interface{})

	Error(interface{})
	Errorf(string, ...interface{})
	Errorln(...interface{})
	ErrorWhen(bool, interface{})

	Warning(interface{})
	Warningf(string, ...interface{})
	Warningln(...interface{})
	WarningWhen(bool, interface{})

	Notice(interface{})
	Noticef(string, ...interface{})
	Noticeln(...interface{})
	NoticeWhen(bool, interface{})

	Info(interface{})
	Infof(string, ...interface{})
	Infoln(...interface{})
	InfoWhen(bool, interface{})

	Debug(interface{})
	Debugf(string, ...interface{})
	Debugln(...interface{})
	DebugWhen(bool, interface{})
}
