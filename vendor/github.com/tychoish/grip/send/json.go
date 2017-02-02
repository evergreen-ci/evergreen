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
	*base
}

// NewJSONConsoleLogger builds a Sender instance that prints log
// messages in a JSON formatted to standard output. The JSON formated
// message is taken by calling the Raw() method on the
// message.Composer and Marshalling the results.
func NewJSONConsoleLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakeJSONConsoleLogger(), name, l)
}

// MakeJSONConsoleLogger returns an un-configured JSON console logging
// instance.
func MakeJSONConsoleLogger() Sender {
	s := &jsonLogger{
		base: newBase(""),
	}

	s.logger = log.New(os.Stdout, "", 0)
	_ = s.SetErrorHandler(ErrorHandlerFromLogger(s.logger))

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

	return setup(s, name, l)
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
	}

	s.closer = func() error {
		return f.Close()
	}

	fallback := log.New(os.Stdout, "", 0)
	if err = s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	s.reset = func() {
		s.logger = log.New(f, "", 0)
		fallback.SetPrefix(fmt.Sprintf("[%s]", s.Name()))
	}

	return s, nil
}

// Implementation of required methods not implemented in Base

func (s *jsonLogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		out, err := json.Marshal(m.Raw())
		if err != nil {
			s.errHandler(err, m)
			return
		}

		s.logger.Println(string(out))
	}
}
