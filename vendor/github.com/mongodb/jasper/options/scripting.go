package options

import (
	"github.com/pkg/errors"
)

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

const (
	GolangScriptingType  = "golang"
	Python2ScriptingType = "python2"
	Python3ScriptingType = "python3"
	RoswellScriptingType = "roswell"
)

// AllScriptingHarnesses returns all supported scripting harnesses.
func AllScriptingHarnesses() map[string]func() ScriptingHarness {
	return map[string]func() ScriptingHarness{
		GolangScriptingType:  func() ScriptingHarness { return &ScriptingGolang{} },
		Python2ScriptingType: func() ScriptingHarness { return &ScriptingPython{LegacyPython: true} },
		Python3ScriptingType: func() ScriptingHarness { return &ScriptingPython{} },
		RoswellScriptingType: func() ScriptingHarness { return &ScriptingRoswell{} },
	}
}

// NewScriptingHarness provides a factory to generate concrete
// implementations of the ScriptingEnvironment interface for use in
// marshaling arbitrary values for a known environment.
func NewScriptingHarness(se string) (ScriptingHarness, error) {
	for harnessName, makeHarness := range AllScriptingHarnesses() {
		if harnessName == se {
			return makeHarness(), nil
		}
	}
	return nil, errors.Errorf("no supported scripting environment named '%s'", se)
}
