package jasper

import (
	"context"
	"syscall"
)

const (
	// EnvironID is the environment variable that is set on all managed
	// processes. The value of this environment variable is always the
	// ID of the process.
	EnvironID = "JASPER_ID"

	// ManagerEnvironID is the environment variable that is set on
	// all managed process that always identifies the process'
	// manager. Used for process tracking and forensics.
	ManagerEnvironID = "JASPER_MANAGER"
)

// Manager provides a basic, high level process management interface
// for processes, and supports creation and introspection. External
// interfaces and remote management tools can be implemented in terms
// of this interface.
type Manager interface {
	CreateProcess(context.Context, *CreateOptions) (Process, error)
	CreateCommand(context.Context) *Command
	Register(context.Context, Process) error

	List(context.Context, Filter) ([]Process, error)
	Group(context.Context, string) ([]Process, error)
	Get(context.Context, string) (Process, error)
	Clear(context.Context)
	Close(context.Context) error
}

// Process objects reflect ways of starting and managing
// processes. Process generally reflect only the primary process at
// the top of a tree and "child" processes are not directly
// reflected. Process implementations either wrap Go's own process
// management calls (e.g. os/exec.Cmd) or may wrap remote process
// management tools (e.g. jasper services on remote systems.)
//
type Process interface {
	// Returns a UUID for the process. Use this ID to retrieve
	// processes from managers using the Get method.
	ID() string

	// Info returns a copy of a structure that reports the current
	// state of the process. If the context is canceled or there
	// is another error, an empty struct may be returned.
	Info(context.Context) ProcessInfo

	// Running provides a quick predicate for checking to see if a
	// process is running. Processes that haven't been started
	// are neither complete nor running.
	Running(context.Context) bool
	// Complete provides a quick predicate for checking if a
	// process has finished. Processes that haven't been started
	// are neither complete nor running.
	Complete(context.Context) bool

	// Signal sends the specified signals to the underlying
	// process. Its error response reflects the outcome of sending
	// the signal, not the state of the process signaled.
	Signal(context.Context, syscall.Signal) error

	// Wait blocks until the process exits or the context is
	// canceled or is not properly defined. Wait will return the
	// exit code as -1 if it was unable to return a true code due
	// to some other error, but otherwise will return the actual
	// exit code of the process. Returns nil if the process has
	// completed successfully.
	//
	// Note that death by signal does not return the signal code
	// and instead is returned as -1.
	Wait(context.Context) (int, error)

	// Respawn respawns a near-identical version of the process on
	// which it is called. It will spawn a new process with the same
	// options and return the new, "respawned" process.
	Respawn(context.Context) (Process, error)

	// RegisterSignalTrigger associates triggers with a process,
	// which execute before the process is about to be signaled.
	RegisterSignalTrigger(context.Context, SignalTrigger) error

	// RegisterSignalTriggerID associates triggers represented by
	// identifiers with a process, which execute before
	// the process is about to be signaled.
	RegisterSignalTriggerID(context.Context, SignalTriggerID) error

	// RegisterTrigger associates triggers with a process,
	// erroring when the context is canceled, the process is
	// complete.
	RegisterTrigger(context.Context, ProcessTrigger) error

	// Tag adds a tag to a process. Implementations should avoid
	// allowing duplicate tags to exist.
	Tag(string)
	// GetTags should return all tags for a process.
	GetTags() []string
	// ResetTags should clear all existing tags.
	ResetTags()
}

// ProcessConstructor is a function type that, given a context.Context and a
// CreateOptions struct, returns a Process and an error.
type ProcessConstructor func(context.Context, *CreateOptions) (Process, error)

// ProcessInfo reports on the current state of a process. It is always
// returned and passed by value, and reflects the state of the process
// when it was created.
type ProcessInfo struct {
	ID         string
	Host       string
	PID        int
	ExitCode   int
	IsRunning  bool
	Successful bool
	Complete   bool
	Timeout    bool
	Options    CreateOptions
}
