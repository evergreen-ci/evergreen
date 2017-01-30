package evergreen

import (
	"bytes"
	"strings"
	"sync"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
	"github.com/tychoish/grip/slogger"
)

var newLine = []byte{'\n'}

// LoggingWriter is a struct - with an associated log
// level severity - that implements io.Writer
type LoggingWriter struct {
	Logger   *slogger.Logger
	Severity level.Priority
	buffer   []byte
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

	// if the logged message does *not* end in a new line, we
	// should buffer it until we find one that does.
	if !bytes.HasSuffix(p, newLine) {
		self.buffer = append(self.buffer, p...)
		return
	}

	// we're ready to write the log message

	// if we had something in the buffer, we should prepend it to
	// the current message, and clear the buffer.
	if len(self.buffer) >= 1 {
		p = append(self.buffer, p...)
		self.buffer = []byte{}
	}

	// Now send each log message:
	lines := bytes.Split(p, newLine)
	for _, val := range lines {
		toString := string(val)
		if strings.Trim(toString, " ") != "" {
			for _, s := range self.Logger.Appenders {
				s.Send(slogger.NewPrefixedLog(self.Logger.Name,
					message.NewDefaultMessage(self.Severity, toString)))
			}
		}
	}
	return len(p), nil
}
