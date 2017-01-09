package send

import (
	"log"
	"os"
	"strings"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

type nativeLogger struct {
	logger *log.Logger
	*base
}

// NewNativeLogger creates a new Sender interface that writes all
// loggable messages to a standard output logger that uses Go's
// standard library logging system.
func NewNativeLogger(name string, l LevelInfo) (Sender, error) {
	s := MakeNative()
	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeNative returns an unconfigured native standard-out logger. You
// *must* call SetName on this instance before using it. (Journaler's
// SetSender will typically do this.)
func MakeNative() Sender {
	s := &nativeLogger{
		base: newBase(""),
	}
	s.level = LevelInfo{level.Trace, level.Trace}

	s.reset = func() {
		s.logger = log.New(os.Stdout, strings.Join([]string{"[", s.Name(), "] "}, ""), log.LstdFlags)
	}

	// we don't call reset here because name isn't set yet, and
	// SetName/SetSender will always call it. The potential for a nil
	// pointer is not 0

	return s
}

func (s *nativeLogger) Type() SenderType { return Native }
func (s *nativeLogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		s.logger.Printf("[p=%s]: %s", m.Priority(), m.Resolve())
	}
}
