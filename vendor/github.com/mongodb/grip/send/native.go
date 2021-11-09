package send

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

type nativeLogger struct {
	logger *log.Logger
	*Base
}

// NewFileLogger creates a Sender implementation that writes log
// output to a file. Returns an error but falls back to a standard
// output logger if there's problems with the file. Internally using
// the go standard library logging system.
func NewFileLogger(name, filePath string, l LevelInfo) (Sender, error) {
	s, err := MakeFileLogger(filePath)
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakeFileLogger creates a file-based logger, writing output to
// the specified file. The Sender instance is not configured: Pass to
// Journaler.SetSender or call SetName before using.
func MakeFileLogger(filePath string) (Sender, error) {
	s := &nativeLogger{Base: NewBase("")}

	if err := s.SetFormatter(MakeDefaultFormatter()); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening logging file, %s", err.Error())
	}

	s.level = LevelInfo{level.Trace, level.Trace}

	s.reset = func() {
		prefix := fmt.Sprintf("[%s] ", s.Name())
		s.logger = log.New(f, prefix, log.LstdFlags)
		_ = s.SetErrorHandler(ErrorHandlerFromLogger(log.New(os.Stderr, prefix, log.LstdFlags)))
	}

	s.closer = func() error {
		return f.Close()
	}

	return s, nil
}

// NewNativeLogger creates a new Sender interface that writes all
// loggable messages to a standard output logger that uses Go's
// standard library logging system.
func NewNativeLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakeNative(), name, l)
}

// MakeNative returns an unconfigured native standard-out logger. You
// *must* call SetName on this instance before using it. (Journaler's
// SetSender will typically do this.)
func MakeNative() Sender {
	return WrapWriterLogger(os.Stdout)
}

// MakeErrorLogger returns an unconfigured Sender implementation that
// writes all logging output to standard error.
func MakeErrorLogger() Sender {
	return WrapWriterLogger(os.Stderr)
}

// NewErrorLogger constructs a configured Sender that writes all
// output to standard error.
func NewErrorLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakeErrorLogger(), name, l)
}

// WrapWriterLogger constructs a new unconfigured sender that directly
// wraps any writer implementation. These loggers prepend time and
// logger name information to the beginning of log lines.
//
// As a special case, if the writer is a *WriterSender, then this
// method will unwrap and return the underlying sender from the writer.
func WrapWriterLogger(wr io.Writer) Sender {
	if s, ok := wr.(*WriterSender); ok {
		return s.Sender
	}

	s := &nativeLogger{
		Base: NewBase(""),
	}
	_ = s.SetFormatter(MakeDefaultFormatter())

	s.level = LevelInfo{level.Trace, level.Trace}

	s.reset = func() {
		s.logger = log.New(wr, fmt.Sprintf("[%s] ", s.Name()), log.LstdFlags)
		_ = s.SetErrorHandler(ErrorHandlerFromLogger(s.logger))
	}

	return s
}

// NewWrappedWriterLogger constructs a fully configured Sender
// implementation that writes all data to the underlying writer.
// These loggers prepend time and logger name information to the
// beginning of log lines.
//
// As a special case, if the writer is a *WriterSender, then this
// method will unwrap and return the underlying sender from the writer.
func NewWrappedWriterLogger(name string, wr io.Writer, l LevelInfo) (Sender, error) {
	return setup(WrapWriterLogger(wr), name, l)
}

// WrapWriter produces a simple writer that does not modify the log
// lines passed to the writer.
//
// As a special case, if the writer is a *WriterSender, then this
// method will unwrap and return the underlying sender from the writer.
func WrapWriter(wr io.Writer) Sender {
	if s, ok := wr.(*WriterSender); ok {
		return s.Sender
	}

	s := &nativeLogger{
		Base: NewBase(""),
	}

	_ = s.SetFormatter(MakePlainFormatter())
	s.level = LevelInfo{level.Trace, level.Trace}
	s.logger = log.New(wr, "", 0)
	return s
}

func (s *nativeLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		out, err := s.formatter(m)
		if err != nil {
			s.ErrorHandler()(err, m)
			return
		}

		s.logger.Print(out)
	}
}

func (s *nativeLogger) Flush(_ context.Context) error { return nil }
