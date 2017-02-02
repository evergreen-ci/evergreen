/*
Call Site Sender

Call site loggers provide a way to record the line number and file
name where the logging call was made, which is particularly useful in
tracing down log messages.

This sender does *not* attach this data to the Message object, and the
call site information is only logged when formatting the message
itself. Additionally the call site includes the file name and its
enclosing directory.

When constructing the Sender you must specify a "depth"
argument This sets the offset for the call site relative to the
Sender's Send() method. Grip's default logger (e.g. the grip.Info()
methods and friends) requires a depth of 2, while in *most* other
cases you will want to use a depth of 1. The LogMany, and
Emergency[Panic,Fatal] methods also include an extra level of
indirection.

Create a call site logger with one of the following constructors:

   NewCallSiteConsoleLogger(<name>, <depth>, <LevelInfo>)
   MakeCallSiteConsoleLogger(<depth>)
   NewCallSiteFileLogger(<name>, <fileName>, <depth>, <LevelInfo>)
   MakeCallSiteFileLogger(<fileName>, <depth>)
*/
package send

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

type callSiteLogger struct {
	depth  int
	logger *log.Logger
	*base
}

// NewCallSiteConsoleLogger returns a fully configured Sender
// implementation that writes log messages to standard output in a
// format that includes the filename and line number of the call site
// of the logger.
func NewCallSiteConsoleLogger(name string, depth int, l LevelInfo) (Sender, error) {
	return setup(MakeCallSiteConsoleLogger(depth), name, l)
}

// MakeCallSiteConsoleLogger constructs an unconfigured call site
// logger that writes output to standard output. You must set the name
// of the logger using SetName or your Journaler's SetSender method
// before using this logger.
func MakeCallSiteConsoleLogger(depth int) Sender {
	s := &callSiteLogger{
		depth: depth,
		base:  newBase(""),
	}

	s.level = LevelInfo{level.Trace, level.Trace}

	s.reset = func() {
		s.logger = log.New(os.Stdout, strings.Join([]string{"[", s.Name(), "] "}, ""), log.LstdFlags)
	}

	return s
}

// NewCallSiteFileLogger returns a fully configured Sender
// implementation that writes log messages to a specified file in a
// format that includes the filename and line number of the call site
// of the logger.
func NewCallSiteFileLogger(name, fileName string, depth int, l LevelInfo) (Sender, error) {
	s, err := MakeCallSiteFileLogger(fileName, depth)
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakeCallSiteFileLogger constructs an unconfigured call site logger
// that writes output to the specified hours. You must set the name of
// the logger using SetName or your Journaler's SetSender method
// before using this logger.
func MakeCallSiteFileLogger(fileName string, depth int) (Sender, error) {
	s := &callSiteLogger{
		depth: depth,
		base:  newBase(""),
	}

	s.level = LevelInfo{level.Trace, level.Trace}

	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening logging file, %s", err.Error())
	}

	s.reset = func() {
		s.logger = log.New(f, strings.Join([]string{"[", s.Name(), "] "}, ""), log.LstdFlags)
	}

	s.closer = func() error {
		return f.Close()
	}

	return s, nil
}

func (s *callSiteLogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		file, line := callerInfo(s.depth)
		s.logger.Printf("[p=%s] [%s:%d]: %s", m.Priority(), file, line, m)
	}
}

func callerInfo(depth int) (string, int) {
	// increase depth to account for callerInfo itself.
	depth++

	// get caller info.
	_, file, line, _ := runtime.Caller(depth)

	// get the directory and filename
	dir, fileName := filepath.Split(file)
	file = filepath.Join(filepath.Base(dir), fileName)

	return file, line
}
