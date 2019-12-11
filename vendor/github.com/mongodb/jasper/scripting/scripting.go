package scripting

import (
	"context"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// Harness provides an interface to execute code in a
// scripting environment such as a Python Virtual
// Environment. Implementations should be make it possible to execute
// either locally or on remote systems.
type Harness interface {
	// ID returns a unique ID for the underlying environment. This
	// should match the ID produced by the underlying options
	// implementation.
	ID() string
	// Setup initializes the environment, and should be safe to
	// call multiple times.
	Setup(context.Context) error
	// Run executes a command (as arguments) with the environment's
	// interpreter.
	Run(context.Context, []string) error
	// RunScript takes the body of a script and should write that
	// data to a file and then runs that script directly.
	RunScript(context.Context, string) error
	// Build will run the environments native build system to
	// generate some kind of build artifact from the scripting
	// environment. Pass a directory in addition to a list of
	// arguments to describe any arguments to the build system.
	// The Build operation returns the path of the build artifact
	// produced by the operation.
	Build(context.Context, string, []string) (string, error)
	// Cleanup should remove the files created by the scripting environment.
	Cleanup(context.Context) error
}

// NewHarness constructs a scripting harness that wraps the
// manager. Use this factory function to build new harnesses, which
// are not cached in the manager (like harnesses constructed directly
// using Manager.CreateScripting), but are otherwise totally functional.
func NewHarness(m jasper.Manager, env options.ScriptingHarness) (Harness, error) {
	if err := env.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	switch t := env.(type) {
	case *options.ScriptingPython:
		return &pythonEnvironment{opts: t, manager: m}, nil
	case *options.ScriptingGolang:
		return &golangEnvironment{opts: t, manager: m}, nil
	case *options.ScriptingRoswell:
		return &roswellEnvironment{opts: t, manager: m}, nil
	default:
		return nil, errors.Errorf("scripting environment %T (%s) is not supported", t, env.Type())
	}
}

// HarnessCache provides an internal local cache for scripting
// environments.
type HarnessCache interface {
	Create(jasper.Manager, options.ScriptingHarness) (Harness, error)
	Get(string) (Harness, error)
	Add(string, Harness) error
	Check(string) bool
}

////////////////////////////////////////////////////////////////////////
//
// internal

type remote interface {
	jasper.Manager
	CreateScripting(context.Context, options.ScriptingHarness) (Harness, error)
	GetScripting(context.Context, string) (Harness, error)
}
