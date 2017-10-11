/*
Stack Messages

The Stack message Composer implementations capture a full stacktrace
information during message construction, and attach a message to that
trace. The string form of the message includes the package and file
name and line number of the last call site, while the Raw form of the
message includes the entire stack. Use with an appropriate sender to
capture the desired output.

All stack message constructors take a "skip" parameter which tells how
many stack frames to skip relative to the invocation of the
constructor. Skip values less than or equal to 0 become 1, and are
equal the call site of the constructor, use larger numbers if you're
wrapping these constructors in our own infrastructure.

In general Composers are lazy, and defer work until the message is
being sent; however, the stack Composers must capture the stack when
they're called rather than when they're sent to produce meaningful
data.
*/
package message

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxLevels = 1024

// types are internal, and exposed only via the composer interface.

type stackMessage struct {
	message string
	tagged  bool
	args    []interface{}
	trace   []StackFrame
	Base
}

// StackFrame captures a single item in a stack trace, and is used
// internally and in the StackTrace output.
type StackFrame struct {
	Function string `bson:"function" json:"function" yaml:"function"`
	File     string `bson:"file" json:"file" yaml:"file"`
	Line     int    `bson:"line" json:"line" yaml:"line"`
}

// StackTrace structs are returned by the Raw method of the stackMessage type
type StackTrace struct {
	Message string       `bson:"message,omitempty" json:"message,omitempty" yaml:"message,omitempty"`
	Frames  []StackFrame `bson:"frames" json:"frames" yaml:"frames"`
	Time    time.Time    `bson:"time" json:"time" yaml:"time"`
}

////////////////////////////////////////////////////////////////////////
//
// Constructors for stack frame messages.
//
////////////////////////////////////////////////////////////////////////

// NewStack builds a Composer implementation that captures the current
// stack trace with a single string message. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func NewStack(skip int, message string) Composer {
	return &stackMessage{
		trace:   captureStack(skip),
		message: message,
	}
}

// NewStackLines returns a composer that builds a fmt.Println style
// message that also captures a stack trace. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func NewStackLines(skip int, messages ...interface{}) Composer {
	return &stackMessage{
		trace: captureStack(skip),
		args:  messages,
	}
}

// NewStackFormatted returns a composer that builds a fmt.Printf style
// message that also captures a stack trace. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func NewStackFormatted(skip int, message string, args ...interface{}) Composer {
	return &stackMessage{
		trace:   captureStack(skip),
		message: message,
		args:    args,
	}
}

////////////////////////////////////////////////////////////////////////
//
// Implementation of Composer methods not implemented by Base
//
////////////////////////////////////////////////////////////////////////

func (m *stackMessage) Loggable() bool { return m.message != "" || len(m.args) > 0 }
func (m *stackMessage) String() string {
	if len(m.args) > 0 && m.message == "" {
		m.message = fmt.Sprintln(append([]interface{}{m.getTag()}, m.args...))
		m.args = []interface{}{}
	} else if len(m.args) > 0 && m.message != "" {
		m.message = fmt.Sprintf(strings.Join([]string{m.getTag(), m.message}, " "), m.args...)
		m.args = []interface{}{}
	} else if !m.tagged {
		m.message = strings.Join([]string{m.getTag(), m.message}, " ")
	}

	return m.message
}

func (m *stackMessage) Raw() interface{} {
	_ = m.Collect()

	return StackTrace{
		Message: m.String(),
		Frames:  m.trace,
		Time:    m.Time,
	}
}

////////////////////////////////////////////////////////////////////////
//
// Internal Operations for Collecting and processing data.
//
////////////////////////////////////////////////////////////////////////

func captureStack(skip int) []StackFrame {
	if skip <= 0 {
		// don't recorded captureStack
		skip = 1
	}

	// captureStack is always called by a constructor, so we need
	// to bump it again
	skip++

	trace := []StackFrame{}

	for i := 0; i < maxLevels; i++ {
		pc, file, line, ok := runtime.Caller(skip)
		if !ok {
			break
		}

		trace = append(trace, StackFrame{
			Function: runtime.FuncForPC(pc).Name(),
			File:     file,
			Line:     line})

		skip++
	}

	return trace
}

func (m *stackMessage) getTag() string {
	if len(m.trace) >= 1 {
		frame := m.trace[0]

		// get the directory and filename
		dir, fileName := filepath.Split(frame.File)

		m.tagged = true

		return fmt.Sprintf("[%s:%d]", filepath.Join(filepath.Base(dir), fileName), frame.Line)
	}

	return ""
}
