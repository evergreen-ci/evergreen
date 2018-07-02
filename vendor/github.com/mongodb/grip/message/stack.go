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
	"go/build"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mongodb/grip/level"
)

const maxLevels = 1024

// types are internal, and exposed only via the composer interface.

type stackMessage struct {
	Composer
	trace stackFrames
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
	Context interface{} `bson:"context,omitempty" json:"context,omitempty" yaml:"context,omitempty"`
	Frames  stackFrames `bson:"frames" json:"frames" yaml:"frames"`
}

func (s StackTrace) String() string { return s.Frames.String() }

////////////////////////////////////////////////////////////////////////
//
// Constructors for stack frame messages.
//
////////////////////////////////////////////////////////////////////////

// WrapStack annotates a message, converted to a composer using the
// normal rules if needed, with a stack trace. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func WrapStack(skip int, msg interface{}) Composer {
	return &stackMessage{
		trace:    captureStack(skip),
		Composer: ConvertToComposer(level.Priority(0), msg),
	}
}

// NewStack builds a Composer implementation that captures the current
// stack trace with a single string message. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func NewStack(skip int, message string) Composer {
	return &stackMessage{
		trace:    captureStack(skip),
		Composer: NewString(message),
	}
}

// NewStackLines returns a composer that builds a fmt.Println style
// message that also captures a stack trace. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func NewStackLines(skip int, messages ...interface{}) Composer {
	return &stackMessage{
		trace:    captureStack(skip),
		Composer: NewLine(messages...),
	}
}

// NewStackFormatted returns a composer that builds a fmt.Printf style
// message that also captures a stack trace. Use the skip argument to
// skip frames if your embedding this in your own wrapper or wrappers.
func NewStackFormatted(skip int, message string, args ...interface{}) Composer {
	return &stackMessage{
		trace:    captureStack(skip),
		Composer: NewFormatted(message, args...),
	}
}

////////////////////////////////////////////////////////////////////////
//
// Implementation of Composer methods not implemented by Base
//
////////////////////////////////////////////////////////////////////////

func (m *stackMessage) String() string {
	return strings.Trim(strings.Join([]string{m.trace.String(), m.Composer.String()}, " "), " \n\t")
}

func (m *stackMessage) Raw() interface{} {
	switch payload := m.Composer.(type) {
	case *fieldMessage:
		payload.fields["stack.frames"] = m.trace
		return payload
	default:
		return StackTrace{
			Context: payload,
			Frames:  m.trace,
		}
	}
}

////////////////////////////////////////////////////////////////////////
//
// Internal Operations for Collecting and processing data.
//
////////////////////////////////////////////////////////////////////////

type stackFrames []StackFrame

func (f stackFrames) String() string {
	out := make([]string, len(f))
	for idx, frame := range f {
		out[idx] = frame.String()
	}

	return strings.Join(out, " ")
}

func (f StackFrame) String() string {
	if strings.HasPrefix(f.File, build.Default.GOPATH) {
		funcNameParts := strings.Split(f.Function, ".")
		var fname string
		if len(funcNameParts) > 0 {
			fname = funcNameParts[len(funcNameParts)-1]
		} else {
			fname = f.Function

		}

		return fmt.Sprintf("%s:%d (%s)",
			f.File[len(build.Default.GOPATH)+5:],
			f.Line,
			fname)
	}

	if strings.HasPrefix(f.File, build.Default.GOROOT) {
		return fmt.Sprintf("%s:%d",
			f.File[len(build.Default.GOROOT)+5:],
			f.Line)
	}

	dir, fileName := filepath.Split(f.File)

	return fmt.Sprintf("%s:%d",
		filepath.Join(filepath.Base(dir), fileName),
		f.Line)
}

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
