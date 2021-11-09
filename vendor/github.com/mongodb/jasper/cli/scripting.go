package cli

import (
	"context"

	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Constants representing the scripting.Harness interface as CLI commands.
const (
	ScriptingCommand          = "scripting"
	ScriptingSetupCommand     = "setup"
	ScriptingRunCommand       = "run"
	ScriptingRunScriptCommand = "run-script"
	ScriptingBuildCommand     = "build"
	ScriptingTestCommand      = "test"
	ScriptingCleanupCommand   = "cleanup"
)

// Scripting creates a cli.Command that supports managing scripting harnesses
// in a remote interface.
func Scripting() cli.Command {
	return cli.Command{
		Name: ScriptingCommand,
		Subcommands: []cli.Command{
			scriptingSetup(),
			scriptingRun(),
			scriptingRunScript(),
			scriptingBuild(),
			scriptingTest(),
			scriptingCleanup(),
		},
	}
}

func scriptingSetup() cli.Command {
	return cli.Command{
		Name:   ScriptingSetupCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := IDInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				sh, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "scripting harness with id '%s' not found", input.ID))
				}
				if err := sh.Setup(ctx); err != nil {
					return makeOutcomeResponse(err)
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func scriptingRun() cli.Command {
	return cli.Command{
		Name:   ScriptingRunCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := ScriptingRunInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				sh, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "scripting harness with id '%s' not found", input.ID))
				}
				if err := sh.Run(ctx, input.Args); err != nil {
					return makeOutcomeResponse(err)
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func scriptingRunScript() cli.Command {
	return cli.Command{
		Name:   ScriptingRunScriptCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := ScriptingRunScriptInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				sh, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "scripting harness with id '%s' not found", input.ID))
				}
				if err := sh.RunScript(ctx, input.Script); err != nil {
					return makeOutcomeResponse(err)
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func scriptingBuild() cli.Command {
	return cli.Command{
		Name:   ScriptingBuildCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := ScriptingBuildInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				sh, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return ScriptingBuildResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "scripting harness with id '%s' not found", input.ID))}
				}
				path, err := sh.Build(ctx, input.Directory, input.Args)
				if err != nil {
					return ScriptingBuildResponse{OutcomeResponse: *makeOutcomeResponse(err)}
				}
				return ScriptingBuildResponse{Path: path, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func scriptingTest() cli.Command {
	return cli.Command{
		Name:   ScriptingTestCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := ScriptingTestInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				sh, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return ScriptingTestResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "scripting harness with id '%s' not found", input.ID))}
				}
				results, err := sh.Test(ctx, input.Directory, input.Options...)
				if err != nil {
					return ScriptingTestResponse{OutcomeResponse: *makeOutcomeResponse(err)}
				}
				return ScriptingTestResponse{Results: results, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func scriptingCleanup() cli.Command {
	return cli.Command{
		Name:   ScriptingCleanupCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := IDInput{}
			return doPassthroughInputOutput(c, &input, func(ctx context.Context, client remote.Manager) interface{} {
				sh, err := client.GetScripting(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "scripting harness with id '%s' not found", input.ID))
				}
				if err := sh.Cleanup(ctx); err != nil {
					return makeOutcomeResponse(err)
				}
				return makeOutcomeResponse(nil)
			})
		},
	}
}
