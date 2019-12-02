package jasper

import (
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

func scriptingEnvironmentFactory(m Manager, env options.ScriptingEnvironment) (ScriptingEnvironment, error) {
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
