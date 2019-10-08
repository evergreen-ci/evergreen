package send

import (
	"errors"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

// InternalSender implements a Sender object that makes it possible to
// access logging messages, in the InternalMessage format without
// logging to an output method. The Send method does not filter out
// under-priority and unloggable messages. Used  for testing
// purposes.
type InternalSender struct {
	name   string
	level  LevelInfo
	output chan *InternalMessage
}

// InternalMessage provides a complete representation of all
// information associated with a logging event.
type InternalMessage struct {
	Message  message.Composer
	Level    LevelInfo
	Logged   bool
	Priority level.Priority
	Rendered string
}

// NewInternalLogger creates and returns a Sender implementation that
// does not log messages, but converts them to the InternalMessage
// format and puts them into an internal channel, that allows you to
// access the massages via the extra "GetMessage" method. Useful for
// testing.
func NewInternalLogger(name string, l LevelInfo) (*InternalSender, error) {
	s := MakeInternalLogger()

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeInternalLogger constructs an internal sender object, typically
// for use in testing.
func MakeInternalLogger() *InternalSender {
	return &InternalSender{
		output: make(chan *InternalMessage, 100),
	}
}

func (s *InternalSender) Name() string                          { return s.name }
func (s *InternalSender) SetName(n string)                      { s.name = n }
func (s *InternalSender) Close() error                          { close(s.output); return nil }
func (s *InternalSender) Level() LevelInfo                      { return s.level }
func (s *InternalSender) SetErrorHandler(_ ErrorHandler) error  { return nil }
func (s *InternalSender) SetFormatter(_ MessageFormatter) error { return nil }
func (s *InternalSender) SetLevel(l LevelInfo) error {
	if !l.Valid() {
		return errors.New("invalid level")
	}

	s.level = l
	return nil
}

// GetMessage pops the first message in the queue and returns.
func (s *InternalSender) GetMessage() *InternalMessage {
	return <-s.output
}

func (s *InternalSender) GetMessageSafe() (*InternalMessage, bool) {
	select {
	case m := <-s.output:
		return m, true
	default:
		return nil, false
	}
}

// HasMessage returns true if there is at least one message that has
// not be removed.
func (s *InternalSender) HasMessage() bool {
	return len(s.output) > 0
}

// Len returns the number of sent messages that have not been retrieved.
func (s *InternalSender) Len() int { return len(s.output) }

// Send sends a message. Unlike all other sender implementations, all
// messages are sent, but the InternalMessage format tracks
// "loggability" for testing purposes.
func (s *InternalSender) Send(m message.Composer) {
	s.output <- &InternalMessage{
		Message:  m,
		Priority: m.Priority(),
		Rendered: m.String(),
		Logged:   s.Level().ShouldLog(m),
	}
}
