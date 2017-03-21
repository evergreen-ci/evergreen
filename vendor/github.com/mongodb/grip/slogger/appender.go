package slogger

import (
	"bytes"
	"os"
	"strings"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
)

// Appender is the slogger equivalent of a send.Sender, and this
// provides the same public interface for Appenders, in terms of Grip's
// senders.
type Appender interface {
	Append(log *Log) error
}

// StdOutAppender returns a configured stream logger Sender instance
// that writes messages to standard output.
func StdOutAppender() send.Sender {
	s, _ := send.NewStreamLogger("", os.Stdout, send.LevelInfo{Default: level.Debug, Threshold: level.Debug})
	return s
}

// StdErrAppender returns a configured stream logger Sender instance
// that writes messages to standard error.
func StdErrAppender() send.Sender {
	s, _ := send.NewStreamLogger("", os.Stderr, send.LevelInfo{Default: level.Debug, Threshold: level.Debug})
	return s
}

// DevNullAppender returns a configured stream logger Sender instance
// that writes messages to dev null.
func DevNullAppender() (send.Sender, error) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}

	return send.NewStreamLogger("", devNull, send.LevelInfo{Default: level.Debug, Threshold: level.Debug})
}

type stringAppender struct {
	buf *bytes.Buffer
}

func (s stringAppender) WriteString(str string) (int, error) {
	if !strings.HasSuffix(str, "\n") {
		str += "\n"
	}
	return s.buf.WriteString(str)
}

// NewStringAppender wraps a bytes.Buffer and provides an
// send.Sender/Appender interface that writes log messages to that
// buffer.
func NewStringAppender(buffer *bytes.Buffer) send.Sender {
	s, _ := send.NewStreamLogger("", stringAppender{buffer}, send.LevelInfo{Default: level.Debug, Threshold: level.Debug})
	return s
}

// LevelFilter provides compatibility with a legacy slogger
// implementation that filgered messages lower than the specified
// priority. This implementation simply sets the threshold level on
// that sender and returns it.
func LevelFilter(threshold Level, sender send.Sender) send.Sender {
	l := sender.Level()
	l.Threshold = threshold.Priority()
	_ = sender.SetLevel(l)

	return sender
}

// SenderAppender is a shim that implements the Appender interface
// around any arbitrary sender instance.
type SenderAppender struct {
	send.Sender
}

// Append sends a log message. This method *always* returns nil.
func (s SenderAppender) Append(log *Log) error {
	s.Send(log.msg)

	return nil
}
