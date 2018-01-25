package send

import (
	"fmt"
	"log"
	"os"

	"github.com/mongodb/grip/level"
)

// NewPlainLogger returns a configured sender that has no prefix and
// uses a plain formatter for messages, using only the string format
// for each message. This sender writes all output to standard output.
func NewPlainLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakePlainLogger(), name, l)
}

// NewPlainErrorLogger returns a configured sender that has no prefix and
// uses a plain formatter for messages, using only the string format
// for each message. This sender writes all output to standard error.
func NewPlainErrorLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakePlainErrorLogger(), name, l)
}

// NewPlainFileLogger creates a new configured logger that writes log
// data to a file.
func NewPlainFileLogger(name, file string, l LevelInfo) (Sender, error) {
	s, err := MakePlainFileLogger(file)
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakePlainLogger returns an unconfigured sender without a prefix,
// using the plain log formatter. This Sender writes all output to
// standard error.
func MakePlainLogger() Sender {
	s := &nativeLogger{
		Base: NewBase(""),
	}
	_ = s.SetFormatter(MakePlainFormatter())

	s.level = LevelInfo{level.Trace, level.Trace}

	s.reset = func() {
		s.logger = log.New(os.Stdout, "", 0)
		_ = s.SetErrorHandler(ErrorHandlerFromLogger(s.logger))
	}

	return s
}

// MakePlainFileLogger writes all output to a file, but does not
// prepend any log formatting to each message.
func MakePlainFileLogger(filePath string) (Sender, error) {
	s := &nativeLogger{Base: NewBase("")}

	if err := s.SetFormatter(MakeDefaultFormatter()); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening logging file, %s", err.Error())
	}

	s.level = LevelInfo{level.Trace, level.Trace}

	fallback := log.New(os.Stderr, "", log.LstdFlags)
	_ = s.SetErrorHandler(ErrorHandlerFromLogger(fallback))

	s.reset = func() {
		s.logger = log.New(f, "", 0)
	}

	s.closer = func() error {
		return f.Close()
	}

	return s, nil
}

// MakePlainErrorLogger returns an unconfigured sender without a prefix,
// using the plain log formatter. This Sender writes all output to
// standard error.
func MakePlainErrorLogger() Sender {
	s := &nativeLogger{
		Base: NewBase(""),
	}
	_ = s.SetFormatter(MakePlainFormatter())

	s.level = LevelInfo{level.Trace, level.Trace}

	s.reset = func() {
		s.logger = log.New(os.Stderr, "", 0)
		_ = s.SetErrorHandler(ErrorHandlerFromLogger(s.logger))
	}

	return s
}
