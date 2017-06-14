package send

import (
	"fmt"
	"log"
	"os"
)

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
	s := &nativeLogger{
		Base:   NewBase(""),
		logger: log.New(os.Stdout, "", 0),
	}

	_ = s.SetFormatter(MakeJSONFormatter())
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
	s := &nativeLogger{Base: NewBase("")}

	if err := s.SetFormatter(MakeJSONFormatter()); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening logging file, %s", err.Error())
	}

	s.logger = log.New(f, "", 0)

	s.closer = func() error {
		return f.Close()
	}

	return s, nil
}
