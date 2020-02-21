package send

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mongodb/grip/message"
)

// this file contains tools to support the slogger interface

// WriteStringer captures the relevant part of the io.Writer interface
// useful for writing log messages to streams.
type WriteStringer interface {
	WriteString(str string) (int, error)
}

type streamLogger struct {
	fobj WriteStringer
	*Base
}

// NewStreamLogger produces a fully configured Sender that writes
// un-formatted log messages to an io.Writer (or conforming subset).
func NewStreamLogger(name string, ws WriteStringer, l LevelInfo) (Sender, error) {
	return setup(MakeStreamLogger(ws), name, l)
}

// MakeStreamLogger constructs an unconfigured stream sender that
// writes un-formatted log messages to the specified io.Writer, or
// instance that implements a conforming subset.
func MakeStreamLogger(ws WriteStringer) Sender {
	s := &streamLogger{
		fobj: ws,
		Base: NewBase(""),
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	_ = s.SetErrorHandler(ErrorHandlerFromLogger(fallback))

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", s.Name()))
	}

	return s
}

func (s *streamLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		msg := m.String()

		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}

		if _, err := s.fobj.WriteString(msg); err != nil {
			s.ErrorHandler()(err, m)
		}
	}
}

func (s *streamLogger) Flush(_ context.Context) error { return nil }
