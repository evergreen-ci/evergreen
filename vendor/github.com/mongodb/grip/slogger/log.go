package slogger

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

const maxStackFrames = 1024

// Log is a representation of a logging event, which matches the
// structure and interface of the original slogger Log
// type. Additionally implements grip's "message.Composer" interface
// for use with other logging mechanisms.
//
// Note that the String() method, which Sender's use to format the
// output of the log lines includes timestamp and component
// (name/prefix) information.
type Log struct {
	Prefix    string    `bson:"prefix,omitempty" json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Level     Level     `bson:"level" json:"level" yaml:"level"`
	Filename  string    `bson:"filename" json:"filename" yaml:"filename"`
	Line      int       `bson:"line" json:"line" yaml:"line"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp" yaml:"timestamp"`
	Output    string    `bson:"message,omitempty" json:"message,omitempty" yaml:"message,omitempty"`
	msg       message.Composer
}

// FormatLog provides compatibility with the original slogger
// implementation.
func FormatLog(log *Log) string {
	return log.String()
}

// Message returns the formatted log message.
func (l *Log) Message() string                        { return l.msg.String() }
func (l *Log) Priority() level.Priority               { return l.Level.Priority() }
func (l *Log) SetPriority(lvl level.Priority) error   { l.Level = convertFromPriority(lvl); return nil }
func (l *Log) Loggable() bool                         { return l.msg.Loggable() }
func (l *Log) Raw() interface{}                       { _ = l.String(); return l }
func (l *Log) Annotate(k string, v interface{}) error { return l.msg.Annotate(k, v) }
func (l *Log) String() string {
	if l.Output == "" {
		year, month, day := l.Timestamp.Date()
		hour, min, sec := l.Timestamp.Clock()

		l.Output = fmt.Sprintf("[%.4d/%.2d/%.2d %.2d:%.2d:%.2d] [%v.%v] [%v:%d] %v",
			year, month, day,
			hour, min, sec,
			l.Prefix, l.Level.String(),
			l.Filename, l.Line,
			l.msg.String())
	}

	return l.Output
}

// NewLog takes a message.Composer object and returns a slogger.Log
// instance (which also implements message.Composer). This method
// records its callsite, so you ought to call this method directly.
func NewLog(m message.Composer) *Log {
	l := &Log{
		Level:     convertFromPriority(m.Priority()),
		Timestamp: time.Now(),
		msg:       m,
	}
	l.appendCallerInfo(2)
	return l
}

func newLog(m message.Composer) *Log {
	l := &Log{
		Level:     convertFromPriority(m.Priority()),
		Timestamp: time.Now(),
		msg:       m,
	}
	l.appendCallerInfo(3)
	return l
}

// NewPrefixedLog allows you to construct a slogger.Log message from a
// message composer, while specifying a prefix.
func NewPrefixedLog(prefix string, m message.Composer) *Log {
	l := NewLog(m)
	l.Prefix = prefix
	l.appendCallerInfo(2)
	return l
}

func newPrefixedLog(prefix string, m message.Composer) *Log {
	l := newLog(m)
	l.Prefix = prefix
	l.appendCallerInfo(3)
	return l

}

func (l *Log) appendCallerInfo(skip int) {
	_, file, line, ok := runtime.Caller(skip)
	if ok {
		l.Filename = stripDirectories(file, 1)
		l.Line = line
	}
}

// These functions are taken directly from the original slogger

func stacktrace() []string {
	ret := make([]string, 0, 2)

	for skip := 2; skip < maxStackFrames; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if !ok {
			break
		}

		ret = append(ret, fmt.Sprintf("at %s:%d", stripDirectories(file, 1), line))
	}

	return ret
}

func stripDirectories(filepath string, toKeep int) string {
	var idxCutoff int
	if idxCutoff = strings.LastIndex(filepath, "/"); idxCutoff == -1 {
		return filepath
	}

outer:
	for dirToKeep := 0; dirToKeep < toKeep; dirToKeep++ {
		switch idx := strings.LastIndex(filepath[:idxCutoff], "/"); idx {
		case -1:
			break outer
		default:
			idxCutoff = idx
		}
	}

	return filepath[idxCutoff+1:]
}
