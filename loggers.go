package evergreen

import (
	"bytes"
	"strings"

	"github.com/10gen-labs/slogger/v1"
)

// LoggingWriter is a struct - with an associated log
// level severity - that implements io.Writer
type LoggingWriter struct {
	Logger   *slogger.Logger
	Severity slogger.Level
}

// NewInfoLoggingWriter is a helper function
// that returns a LoggingWriter for information logs
func NewInfoLoggingWriter(logger *slogger.Logger) *LoggingWriter {
	return &LoggingWriter{
		Logger:   logger,
		Severity: slogger.INFO,
	}
}

// NewErrorLoggingWriter is a helper function
// that returns a LoggingWriter for errors
func NewErrorLoggingWriter(logger *slogger.Logger) *LoggingWriter {
	return &LoggingWriter{
		Logger:   logger,
		Severity: slogger.ERROR,
	}
}

// Since LoggingWriter is an io.Writer,
// it must implement the Write function
func (self *LoggingWriter) Write(p []byte) (n int, err error) {
	lines := bytes.Split(p, []byte{'\n'})
	for _, val := range lines {
		toString := string(val)
		if strings.Trim(toString, " ") != "" {
			self.Logger.Logf(self.Severity, "%s", toString)
		}
	}
	return len(p), nil
}
