package cli

import (
	"context"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Manager creates a cli.Command that interfaces with a Jasper manager. Each
// subcommand optionally reads the arguments as JSON from stdin if any are
// required, calls the jasper.Manager function corresponding to that subcommand,
// and writes the response as JSON to stdout.
func Manager() cli.Command {
	return cli.Command{
		Name: "manager",
		Subcommands: []cli.Command{
			managerCreateProcess(),
			managerCreateCommand(),
			managerGet(),
			managerList(),
			managerGroup(),
			managerClear(),
			managerClose(),
		},
	}
}

func managerCreateProcess() cli.Command {
	return cli.Command{
		Name:   "create-process",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			opts := &jasper.CreateOptions{}
			return doPassthroughInputOutput(c, opts, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.CreateProcess(ctx, opts)
				if err != nil {
					return &InfoResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error creating process"))}
				}
				return &InfoResponse{Info: proc.Info(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func managerCreateCommand() cli.Command {
	return cli.Command{
		Name:   "create-command",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			opts := &CommandInput{}
			return doPassthroughInputOutput(c, opts, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				cmd := client.CreateCommand(ctx).Background(opts.Background)
				if level.IsValidPriority(opts.Priority) {
					cmd = cmd.Priority(opts.Priority)
				}
				cmd = cmd.Extend(opts.Commands)
				cmd = cmd.Background(opts.Background).
					ContinueOnError(opts.ContinueOnError).
					IgnoreError(opts.IgnoreError).
					ApplyFromOpts(&opts.CreateOptions)
				return makeOutcomeResponse(cmd.Run(ctx))
			})
		},
	}
}

func managerGet() cli.Command {
	return cli.Command{
		Name:   "get",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
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
		Name:   "list",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &FilterInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
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
		Name:   "group",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &TagInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
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
		Name:   "clear",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				client.Clear(ctx)
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func managerClose() cli.Command {
	return cli.Command{
		Name:   "close",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			return doPassthroughOutput(c, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				return makeOutcomeResponse(client.Close(ctx))
			})
		},
	}
}
