package evergreen

import (
	"bytes"
	"github.com/10gen-labs/slogger/v1"
	"strings"
)

//SliceAppender is a slogger.Appender implemenation that just adds every
//log message to an internal slice. Useful for testing when a test needs to
//capture data sent to a slogger.Logger and verify what data was written.
type SliceAppender struct {
	Messages []*slogger.Log
}

func (self *SliceAppender) Append(log *slogger.Log) error {
	self.Messages = append(self.Messages, log)
	return nil
}

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
