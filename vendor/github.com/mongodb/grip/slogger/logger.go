package slogger

import (
	"errors"

	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

// Logger is a type that represents a single log instance. This is
// analogous to the original slogger's Logger type; however, the
// Appender field stores a slice of sned.Senders rather than
// slogger.Appenders; however, the Appender type is defined in the new
// slogger, and the NewAppenderSender and WrapAppender functions
// afford the conversion.
type Logger struct {
	Name      string
	Appenders []send.Sender
}

// Logf logs a message and a level to a logger instance. This returns a
// pointer to a Log and a slice of errors that were gathered from every
// Appender (nil errors included).
//
// In this implementation, Logf will never return an error. This part
// of the type definition was retained for backwards compatibility
func (l *Logger) Logf(level Level, messageFmt string, args ...interface{}) (*Log, []error) {
	m := newPrefixedLog(l.Name, message.NewFormattedMessage(level.Priority(), messageFmt, args...))

	for _, send := range l.Appenders {
		send.Send(m)
	}

	return m, []error{}
}

// Errorf logs and returns its message as an error.
//
// Log and return a formatted error string.
// Example:i
//
// if whatIsExpected != whatIsReturned {
//     return slogger.Errorf(slogger.WARN, "Unexpected return value. Expected: %v Received: %v",
//         whatIsExpected, whatIsReturned)
// }
func (l *Logger) Errorf(level Level, messageFmt string, args ...interface{}) error {
	m := message.NewFormattedMessage(level.Priority(), messageFmt, args...)
	log := newPrefixedLog(l.Name, m)

	for _, send := range l.Appenders {
		send.Send(log)
	}

	return errors.New(m.String())
}

// Stackf is designed to work in tandem with `NewStackError`. This
// function is similar to `Logf`, but takes a `stackErr`
// parameter. `stackErr` is expected to be of type StackError, but does
// not have to be.
//
// In this implementation, Logf will never return an error. This part
// of the type definition was retained for backwards compatibility.
//
// An additional difference is that the new implementation, will not
// log if the error is nil, and the previous implementation would.
func (l *Logger) Stackf(level Level, stackErr error, messageFmt string, args ...interface{}) (*Log, []error) {
	m := newPrefixedLog(l.Name, message.NewErrorWrapMessage(level.Priority(), stackErr, messageFmt, args...))

	// this check is not necessary, but prevents redundancy if
	// there are multiple senders, as each might need to perform this
	// check for themselves.
	if !m.Loggable() {
		return m, []error{}
	}

	for _, send := range l.Appenders {
		send.Send(m)
	}

	return m, []error{}
}
