package jasper

import (
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// NewScriptingHarness constructs a scripting harness that wraps the
// manager. Use this factory function to build new harnesses, which
// are not cached in the manager (like harnesses constructed directly
// using Manager.CreateScripting), but are otherwise totally functional.
func NewScriptingHarness(m Manager, env options.ScriptingHarness) (ScriptingHarness, error) {
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
