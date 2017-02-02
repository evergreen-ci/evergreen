package send

import (
	"errors"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

// internalSender implements a Sender object that makes it possible to
// access logging messages, in the InternalMessage format without
// logging to an output method. The Send method does not filter out
// under-priority and unloggable messages. Used  for testing
// purposes.
type internalSender struct {
	name   string
	level  LevelInfo
	output chan *internalMessage
}

// InternalMessage provides a complete representation of all
// information associated with a logging event.
type internalMessage struct {
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
func NewInternalLogger(name string, l LevelInfo) (*internalSender, error) {
	s := MakeInternalLogger()

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeInternalLogger constructs an internal sender object, typically
// for use in testing.
func MakeInternalLogger() *internalSender {
	return &internalSender{
		output: make(chan *internalMessage, 100),
	}
}

func (s *internalSender) Name() string                         { return s.name }
func (s *internalSender) SetName(n string)                     { s.name = n }
func (s *internalSender) Close() error                         { close(s.output); return nil }
func (s *internalSender) Level() LevelInfo                     { return s.level }
func (s *internalSender) SetErrorHandler(_ ErrorHandler) error { return nil }
func (s *internalSender) SetLevel(l LevelInfo) error {
	if !l.Valid() {
		return errors.New("invalid level")
	}

	s.level = l
	return nil
}
func (s *internalSender) GetMessage() *internalMessage {
	return <-s.output
}

func (s *internalSender) Len() int {
	return len(s.output)
}

func (s *internalSender) Send(m message.Composer) {
	s.output <- &internalMessage{
		Message:  m,
		Priority: m.Priority(),
		Rendered: m.String(),
		Logged:   s.level.ShouldLog(m),
	}
}
