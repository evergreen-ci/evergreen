// Package send provides an interface for defining "senders" for
// different logging backends, as well as basic implementations for
// common logging approaches to use with the Grip logging
// interface. Backends currently include: syslog, systemd's journal,
// standard output, and file baased methods.
package send

import (
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

// The Sender interface describes how the Journaler type's method in
// primary "grip" package's methods interact with a logging output
// method. The Journaler type provides Sender() and SetSender()
// methods that allow client code to swap logging backend
// implementations dependency-injection style.
type Sender interface {
	// returns the name of the logging system. Typically this corresponds directly with
	Name() string
	SetName(string)

	// returns a constant for the type of the sender. Used by the
	// loggers as part of their dependency injection mechanism.
	Type() SenderType

	// Method that actually sends messages (the string) to the
	// logging capture system. The Send() method filters out
	// logged messages based on priority, typically using the
	// generic MessageInfo.ShouldLog() function.
	Send(message.Composer)

	// SetLevel allows you to modify the level
	// configuration. Returns an error if you specify impossible
	// values.
	SetLevel(LevelInfo) error

	// Level returns the level configuration document.
	Level() LevelInfo

	// If the logging sender holds any resources that require
	// desecration, they should be cleaned up tin the Close()
	// method. Close() is called by the SetSender() method before
	// changing loggers.
	Close() error
}

// LevelInfo provides a sender-independent structure for storing
// information about a sender's configured log levels.
type LevelInfo struct {
	Default   level.Priority
	Threshold level.Priority
}

// Valid checks that the priorities stored in the LevelInfo document are valid.
func (l LevelInfo) Valid() bool {
	return level.IsValidPriority(l.Default) && level.IsValidPriority(l.Threshold)
}

// ShouldLog checks to see if the log message should be logged, and
// returns false if there is no message or if the message's priority
// is below the logging threshold.
func (l LevelInfo) ShouldLog(m message.Composer) bool {
	// priorities are 0 = Emergency; 7 = debug
	return m.Loggable() && (m.Priority() >= l.Threshold)
}
