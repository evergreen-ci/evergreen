// Package send provides an interface for defining "senders" for
// different logging backends, as well as basic implementations for
// common logging approaches to use with the Grip logging
// interface. Backends currently include: syslog, systemd's journal,
// standard output, and file baased methods.
package send

import (
	"context"
	"log"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

// The Sender interface describes how the Journaler type's method in primary
// "grip" package's methods interact with a logging output method. The
// Journaler type provides Sender() and SetSender() methods that allow client
// code to swap logging backend implementations dependency-injection style.
type Sender interface {
	// Name returns the name of the logging system. Typically this
	// corresponds directly with the underlying logging capture system.
	Name() string
	//SetName sets the name of the logging system.
	SetName(string)

	// Method that actually sends messages (the string) to the logging
	// capture system. The Send() method filters out logged messages based
	// based on priority, typically using the generic
	// MessageInfo.ShouldLog() function.
	Send(message.Composer)

	// Flush flushes any potential buffered messages to the logging capture
	// system. If the Sender is not buffered, this function should noop and
	// return nil.
	Flush(context.Context) error

	// SetLevel allows you to modify the level configuration. Returns an
	// error if you specify impossible values.
	SetLevel(LevelInfo) error

	// Level returns the level configuration document.
	Level() LevelInfo

	// SetErrorHandler provides a method to inject error handling behavior
	// to a sender. Not all sender implementations use the error handler,
	// although some, use a default handler to write logging errors to
	// standard output.
	SetErrorHandler(ErrorHandler) error
	ErrorHandler() ErrorHandler

	// SetFormatter allows users to inject formatting functions to modify
	// the output of the log sender by providing a function that takes a
	// message and returns string and error.
	SetFormatter(MessageFormatter) error
	Formatter() MessageFormatter

	// If the logging sender holds any resources that require desecration
	// they should be cleaned up in the Close() method. Close() is called
	// by the SetSender() method before changing loggers.
	Close() error
}

// LevelInfo provides a sender-independent structure for storing information
// about a sender's configured log levels.
type LevelInfo struct {
	Default   level.Priority `json:"default" bson:"default"`
	Threshold level.Priority `json:"threshold" bson:"threshold"`
}

// Valid checks that the priorities stored in the LevelInfo document are valid.
func (l LevelInfo) Valid() bool { return l.Default.IsValid() && l.Threshold.IsValid() }

// ShouldLog checks to see if the log message should be logged, and returns
// false if there is no message or if the message's priority is below the
// logging threshold.
func (l LevelInfo) ShouldLog(m message.Composer) bool {
	// priorities are 0 = Emergency; 7 = debug
	return m.Loggable() && (m.Priority() >= l.Threshold)
}

func setup(s Sender, name string, l LevelInfo) (Sender, error) {
	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeStandardLogger creates a standard library logging instance that logs all
// messages to the underlying sender directly at the specified level.
func MakeStandardLogger(s Sender, p level.Priority) *log.Logger {
	return log.New(MakeWriterSender(s, p), "", 0)
}
