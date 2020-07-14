package executor

import (
	"io"
	"syscall"
)

// Executor is an interface by which Jasper processes can manipulate and
// introspect on processes. Implementations are not guaranteed to be
// thread-safe.
type Executor interface {
	// Args returns the command and the arguments used to create the process.
	Args() []string
	// SetEnv sets the process environment.
	SetEnv([]string)
	// Env returns the process environment.
	Env() []string
	// SetDir sets the process working directory.
	SetDir(string)
	// Dir returns the process working directory.
	Dir() string
	// SetStdin sets the process standard input.
	SetStdin(io.Reader)
	// SetStdout sets the process standard output.
	SetStdout(io.Writer)
	// Stdout returns the process standard output.
	Stdout() io.Writer
	// SetStderr sets the process standard error.
	SetStderr(io.Writer)
	// Stderr returns the process standard error.
	Stderr() io.Writer
	// Start begins execution of the process.
	Start() error
	// Wait waits for the process to complete.
	Wait() error
	// Signal sends a signal to a running process.
	Signal(syscall.Signal) error
	// PID returns the local process ID of the process if it is running or
	// complete. This is not guaranteed to return a valid value for remote
	// executors and will return -1 if it could not be retrieved.
	PID() int
	// ExitCode returns the exit code of a completed process. This will return a
	// non-negative value if it successfully retrieved the exit code. Callers
	// must call Wait before retrieving the exit code.
	ExitCode() int
	// Success returns whether or not the completed process ran successfully.
	// Callers must call Wait before checking for success.
	Success() bool
	// SignalInfo returns information about signals the process has received.
	SignalInfo() (sig syscall.Signal, signaled bool)
	// Close cleans up the executor's resources. Users should not assume the
	// information from the Executor will be accurate after it has been closed.
	Close() error
}

// Status represents the current state of the Executor.
type Status int

func (s Status) String() string {
	switch s {
	case Unknown:
		return "unknown"
	case Unstarted:
		return "unstarted"
	case Running:
		return "running"
	case Exited:
		return "exited"
	case Closed:
		return "closed"
	default:
		return ""
	}
}

// Before returns whether or not the given state occurs before the current
// state.
func (s Status) Before(other Status) bool {
	return s < other
}

// BeforeInclusive is the same as Before but also returns true if the given
// state is identical to the current state.
func (s Status) BeforeInclusive(other Status) bool {
	return s <= other
}

// After returns whether or not the given state occurs before the current state.
func (s Status) After(other Status) bool {
	return s > other
}

// AfterInclusive is the same as After but also returns true if the given state
// is identical to the current state.
func (s Status) AfterInclusive(other Status) bool {
	return s >= other
}

// Between returns whether or not the state occurs after the given lower state
// and after the given upper state.
func (s Status) Between(lower Status, upper Status) bool {
	return lower < s && s < upper
}

// BetweenInclusive is the same as Between but also returns true if the given
// state is identical to the given lower or upper state.
func (s Status) BetweenInclusive(lower Status, upper Status) bool {
	return lower <= s && s <= upper
}

const (
	// Unknown means the process is in an unknown or invalid state.
	Unknown Status = iota
	// Unstarted means that the Executor has not yet started running the
	// process.
	Unstarted Status = iota
	// Running means that the Executor has started running the process.
	Running Status = iota
	// Exited means the Executor has finished running the process.
	Exited Status = iota
	// Closed means the Executor has cleaned up its resources and further
	// requests cannot be made.
	Closed Status = iota
)
