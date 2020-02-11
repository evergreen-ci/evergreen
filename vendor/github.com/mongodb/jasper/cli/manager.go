package cli

import (
	"context"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Constants representing the Jasper Manager interface as CLI commands.
const (
	ManagerCommand         = "manager"
	IDCommand              = "id"
	CreateProcessCommand   = "create-process"
	CreateScriptingCommand = "create-scripting"
	CreateCommand          = "create-command"
	GetCommand             = "get"
	GroupCommand           = "group"
	ListCommand            = "list"
	ClearCommand           = "clear"
	CloseCommand           = "close"
)

// Manager creates a cli.Command that interfaces with a Jasper manager. Each
// subcommand optionally reads the arguments as JSON from stdin if any are
// required, calls the jasper.Manager function corresponding to that subcommand,
// and writes the response as JSON to stdout.
func Manager() cli.Command {
	return cli.Command{
		Name: ManagerCommand,
		Subcommands: []cli.Command{
			managerID(),
			managerCreateProcess(),
			managerCreateCommand(),
			managerGet(),
			managerList(),
			managerGroup(),
			managerClear(),
			managerClose(),
			managerCreateScripting(),
		},
	}
}

func managerID() cli.Command {
	return cli.Command{
		Name:   IDCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client remote.Manager) interface{} {
				id := client.ID()
				return &IDResponse{OutcomeResponse: *makeOutcomeResponse(nil), ID: id}
			})
		},
	}
}

func managerCreateProcess() cli.Command {
	return cli.Command{
		Name:   CreateProcessCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			opts := &options.Create{}
			return doPassthroughInputOutput(c, opts, func(ctx context.Context, client remote.Manager) interface{} {
				proc, err := client.CreateProcess(ctx, opts)
				if err != nil {
					return &InfoResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error creating process"))}
				}
				return &InfoResponse{Info: proc.Info(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func managerCreateScripting() cli.Command {
	return cli.Command{
		Name:   CreateScriptingCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			opts := &ScriptingOptions{}
			return doPassthroughInputOutput(c, opts, func(ctx context.Context, client remote.Manager) interface{} {
				harnessOpts, err := opts.Export()
				if err != nil {
					return &IDResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error creating scripting harness"))}
				}

				env, err := client.CreateScripting(ctx, harnessOpts)
				if err != nil {
					return &IDResponse{ID: harnessOpts.ID(), OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error creating scripting harness"))}
				}
				return &IDResponse{ID: env.ID(), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func managerCreateCommand() cli.Command {
	return cli.Command{
		Name:   CreateCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			opts := &options.Command{}
			return doPassthroughInputOutput(c, opts, func(ctx context.Context, client remote.Manager) interface{} {
				cmd := client.CreateCommand(ctx).Extend(opts.Commands).
					Background(opts.RunBackground).
					ContinueOnError(opts.ContinueOnError).
					IgnoreError(opts.IgnoreError).
					Sudo(opts.Sudo).
					ApplyFromOpts(&opts.Process)
				if opts.SudoUser != "" {
					cmd.SudoAs(opts.SudoUser)
				}
				if opts.Priority == 0 || opts.Priority.IsValid() {
					cmd = cmd.Priority(opts.Priority)
				}
				return makeOutcomeResponse(cmd.Run(ctx))
			})
		},
	}
}

func managerGet() cli.Command {
	return cli.Command{
		Name:   GetCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client remote.Manager) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &InfoResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error getting process with ID '%s'", input.ID))}
				}
				return &InfoResponse{Info: proc.Info(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func managerList() cli.Command {
	return cli.Command{
		Name:   ListCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &FilterInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client remote.Manager) interface{} {
				procs, err := client.List(ctx, input.Filter)
				if err != nil {
					return &InfosResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error listing processes with filter '%s'", input.Filter))}
				}
				infos := make([]jasper.ProcessInfo, 0, len(procs))
				for _, proc := range procs {
					infos = append(infos, proc.Info(ctx))
				}
				return &InfosResponse{Infos: infos, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func managerGroup() cli.Command {
	return cli.Command{
		Name:   GroupCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &TagInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client remote.Manager) interface{} {
				procs, err := client.Group(ctx, input.Tag)
				if err != nil {
					return &InfosResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error grouping processes with tag '%s'", input.Tag))}
				}
				infos := make([]jasper.ProcessInfo, 0, len(procs))
				for _, proc := range procs {
					infos = append(infos, proc.Info(ctx))
				}
				return &InfosResponse{Infos: infos, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func managerClear() cli.Command {
	return cli.Command{
		Name:   ClearCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client remote.Manager) interface{} {
				client.Clear(ctx)
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func managerClose() cli.Command {
	return cli.Command{
		Name:   CloseCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client remote.Manager) interface{} {
				return makeOutcomeResponse(client.Close(ctx))
			})
		},
	}
}
