package slogger

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type Log struct {
	Prefix     string
	Level      Level
	Filename   string
	Line       int
	Timestamp  time.Time
	messageFmt string
	args       []interface{}
}

func (self *Log) Message() string {
	return fmt.Sprintf(self.messageFmt, self.args...)
}

type Logger struct {
	Prefix    string
	Appenders []Appender
}

// Log a message and a level to a logger instance. This returns a
// pointer to a Log and a slice of errors that were gathered from every
// Appender (nil errors included).
func (self *Logger) Logf(level Level, messageFmt string, args ...interface{}) (*Log, []error) {
	return self.logf(level, messageFmt, args...)
}

// Log and return a formatted error string.
// Example:
//
// if whatIsExpected != whatIsReturned {
//     return slogger.Errorf(slogger.WARN, "Unexpected return value. Expected: %v Received: %v",
//         whatIsExpected, whatIsReturned)
// }
//
func (self *Logger) Errorf(level Level, messageFmt string, args ...interface{}) error {
	log, _ := self.logf(level, messageFmt, args...)
	return errors.New(log.Message())
}

// Stackf is designed to work in tandem with `NewStackError`. This
// function is similar to `Logf`, but takes a `stackErr`
// parameter. `stackErr` is expected to be of type StackError, but does
// not have to be.
func (self *Logger) Stackf(level Level, stackErr error, messageFmt string, args ...interface{}) (*Log, []error) {
	messageFmt = fmt.Sprintf("%v\n%v", messageFmt, stackErr.Error())
	return self.logf(level, messageFmt, args...)
}

func (self *Logger) logf(level Level, messageFmt string, args ...interface{}) (*Log, []error) {
	var errors []error

	_, file, line, ok := runtime.Caller(2)
	if ok == false {
		return nil, []error{fmt.Errorf("Failed to find the calling method.")}
	}

	file = stripDirectories(file, 2)

	log := &Log{
		Prefix:     self.Prefix,
		Level:      level,
		Filename:   file,
		Line:       line,
		Timestamp:  time.Now(),
		messageFmt: messageFmt,
		args:       args,
	}

	Cache.Add(log)

	for _, appender := range self.Appenders {
		if err := appender.Append(log); err != nil {
			error := fmt.Errorf("Error appending. Appender: %T Error: %v", appender, err)
			errors = append(errors, error)
		}
	}

	return log, errors
}

type Level uint8

// The level is in an order such that the expressions
// `level < WARN`, `level >= INFO` have intuitive meaning.
const (
	OFF Level = iota
	DEBUG
	INFO
	WARN
	ERROR
)

func (self Level) Type() string {
	switch self {
	case ERROR:
		return "error"
	case WARN:
		return "warn"
	case INFO:
		return "info"
	case DEBUG:
		return "debug"
	}

	return "off?"
}

func stacktrace() []string {
	ret := make([]string, 0, 2)
	for skip := 2; true; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if ok == false {
			break
		}

		ret = append(ret, fmt.Sprintf("at %s:%d", stripDirectories(file, 2), line))
	}

	return ret
}

type StackError struct {
	Message    string
	Stacktrace []string
}

func NewStackError(messageFmt string, args ...interface{}) *StackError {
	return &StackError{
		Message:    fmt.Sprintf(messageFmt, args...),
		Stacktrace: stacktrace(),
	}
}

func (self *StackError) Error() string {
	return fmt.Sprintf("%s\n\t%s", self.Message, strings.Join(self.Stacktrace, "\n\t"))
}

func stripDirectories(filepath string, toKeep int) string {
	var idxCutoff int
	if idxCutoff = strings.LastIndex(filepath, "/"); idxCutoff == -1 {
		return filepath
	}

	for dirToKeep := 0; dirToKeep < toKeep; dirToKeep++ {
		switch idx := strings.LastIndex(filepath[:idxCutoff], "/"); idx {
		case -1:
			break
		default:
			idxCutoff = idx
		}
	}

	return filepath[idxCutoff+1:]
}
