package options

import "github.com/pkg/errors"

// ScriptingHarness defines the interface for all types that
// define a scripting environment.
type ScriptingHarness interface {
	// ID should return a unique hash of the implementation of
	// ScrptingEnvironment. This can be cached, and should change
	// if any of the dependencies change.
	ID() string
	// Type returns the name of the environment, and is useful to
	// identify the environment for users.
	Type() string
	// Interpreter should return a path to the binary that will be
	// used for running code.
	Interpreter() string
	// Validate checks the internal consistency of an
	// implementation and may set defaults.
	Validate() error
}

// NewScriptingHarness provides a factory to generate concrete
// implementations of the ScriptingEnvironment interface for use in
// marshaling arbitrary values for a known environment.
func NewScriptingHarness(se string) (ScriptingHarness, error) {
	switch se {
	case "python2":
		return &ScriptingPython{LegacyPython: true}, nil
	case "python", "python3":
		return &ScriptingPython{LegacyPython: false}, nil
	case "go", "golang":
		return &ScriptingGolang{}, nil
	case "roswell", "ros", "lisp", "cl":
		return &ScriptingRoswell{}, nil
	default:
		return nil, errors.Errorf("no supported scripting environment named '%s'", se)
	}
}
