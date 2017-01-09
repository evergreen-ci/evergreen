package send

import (
	"strings"

	"github.com/tychoish/grip/message"
)

// this file contains tools to support the slogger interface

// WriteStringer captures the relevant part of the io.Writer interface
// useful for writing log messages to streams.
type WriteStringer interface {
	WriteString(str string) (int, error)
}

type streamLogger struct {
	fobj WriteStringer
	*base
}

// NewStreamLogger produces a fully configured Sender that writes
// un-formatted log messages to an io.Writer (or conforming subset).
func NewStreamLogger(name string, ws WriteStringer, l LevelInfo) (Sender, error) {
	s := MakeStreamLogger(ws)

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeStreamLogger constructs an unconfigured stream sender that
// writes un-formatted log messages to the specified io.Writer, or
// instance that implements a conforming subset.
func MakeStreamLogger(ws WriteStringer) Sender {
	return &streamLogger{
		fobj: ws,
		base: newBase(""),
	}
}

func (s *streamLogger) Type() SenderType { return Stream }
func (s *streamLogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		msg := m.Resolve()

		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}

		_, _ = s.fobj.WriteString(msg)
	}
}
