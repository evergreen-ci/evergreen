package evergreen

import (
	"bytes"
	"strings"
	"sync"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
	"github.com/tychoish/grip/slogger"
)

// LoggingWriter is a struct - with an associated log
// level severity - that implements io.Writer
type LoggingWriter struct {
	Logger   *slogger.Logger
	Severity level.Priority
	mutex    sync.Mutex
}

// NewInfoLoggingWriter is a helper function
// that returns a LoggingWriter for information logs
func NewInfoLoggingWriter(logger *slogger.Logger) *LoggingWriter {
	return &LoggingWriter{
		Logger:   logger,
		Severity: level.Info,
	}
}

// NewErrorLoggingWriter is a helper function
// that returns a LoggingWriter for errors
func NewErrorLoggingWriter(logger *slogger.Logger) *LoggingWriter {
	return &LoggingWriter{
		Logger:   logger,
		Severity: level.Error,
	}
}

// Since LoggingWriter is an io.Writer,
// it must implement the Write function
func (self *LoggingWriter) Write(p []byte) (n int, err error) {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	lines := bytes.Split(p, []byte{'\n'})
	for _, val := range lines {
		toString := string(val)
		if strings.Trim(toString, " ") != "" {
			for _, s := range self.Logger.Appenders {
				s.Send(message.NewDefaultMessage(self.Severity, toString))
			}
		}
	}
	return len(p), nil
}
