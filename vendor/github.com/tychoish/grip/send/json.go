package send

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/tychoish/grip/message"
)

type jsonLogger struct {
	logger *log.Logger
	closer func() error
	*base
}

// NewJSONConsoleLogger builds a Sender instance that prints log
// messages in a JSON formatted to standard output. The JSON formated
// message is taken by calling the Raw() method on the
// message.Composer and Marshalling the results.
func NewJSONConsoleLogger(name string, l LevelInfo) (Sender, error) {
	s := MakeJSONConsoleLogger()
	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeJSONConsoleLogger returns an un-configured JSON console logging
// instance.
func MakeJSONConsoleLogger() Sender {
	s := &jsonLogger{
		base:   newBase(""),
		closer: func() error { return nil },
	}

	s.reset = func() {
		s.logger = log.New(os.Stdout, "", 0)
	}

	return s
}

// NewJSONFileLogger builds a Sender instance that write JSON
// formated log messages to a file, with one-line per message. The
// JSON formated message is taken by calling the Raw() method on the
// message.Composer and Marshalling the results.
func NewJSONFileLogger(name, file string, l LevelInfo) (Sender, error) {
	s, err := MakeJSONFileLogger(file)
	if err != nil {
		return nil, err
	}

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeJSONFileLogger creates an un-configured JSON logger that writes
// output to the specified file.
func MakeJSONFileLogger(file string) (Sender, error) {
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening logging file, %s", err.Error())
	}

	s := &jsonLogger{
		base: newBase(""),
		closer: func() error {
			return f.Close()
		},
	}

	s.reset = func() {
		s.logger = log.New(f, "", 0)
	}

	return s, nil
}

// Implementation of required methods not implemented in BASE

func (s *jsonLogger) Type() SenderType { return JSON }
func (s *jsonLogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		out, err := json.Marshal(m.Raw())
		if err != nil {
			errMsg, _ := json.Marshal(message.NewError(err).Raw())
			s.logger.Println(errMsg)

			out, err = json.Marshal(message.NewDefaultMessage(m.Priority(), m.Resolve()).Raw())
			if err == nil {
				s.logger.Println(string(out))
			}

			return
		}

		s.logger.Println(string(out))
	}
}
