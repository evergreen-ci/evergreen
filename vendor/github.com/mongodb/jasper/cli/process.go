package cli

import (
	"context"
	"syscall"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Process creates a cli.Command that interfaces with a Jasper process.
func Process() cli.Command {
	return cli.Command{
		Name: "process",
		Subcommands: []cli.Command{
			processInfo(),
			processRunning(),
			processComplete(),
			processSignal(),
			processWait(),
			processRespawn(),
			processRegisterSignalTriggerID(),
			processTag(),
			processGetTags(),
			processResetTags(),
		},
	}
}

func processInfo() cli.Command {
	return cli.Command{
		Name:   "info",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &InfoResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))}
				}
				return &InfoResponse{Info: proc.Info(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func processRunning() cli.Command {
	return cli.Command{
		Name:   "running",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &RunningResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))}
				}
				return &RunningResponse{Running: proc.Running(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func processComplete() cli.Command {
	return cli.Command{
		Name:   "complete",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &CompleteResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))}
				}
				return &CompleteResponse{Complete: proc.Complete(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func processSignal() cli.Command {
	return cli.Command{
		Name: "signal",
		Action: func(c *cli.Context) error {
			input := &SignalInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))
				}
				return makeOutcomeResponse(proc.Signal(ctx, syscall.Signal(input.Signal)))
			})
		},
	}
}

func processWait() cli.Command {
	return cli.Command{
		Name:   "wait",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &WaitResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))}
				}
				exitCode, err := proc.Wait(ctx)
				if err != nil {
					return &WaitResponse{ExitCode: exitCode, Error: err.Error(), OutcomeResponse: *makeOutcomeResponse(nil)}
				}
				return &WaitResponse{ExitCode: exitCode, OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func processRespawn() cli.Command {
	return cli.Command{
		Name: "respawn",
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &InfoResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))}
				}
				newProc, err := proc.Respawn(ctx)
				if err != nil {
					return &InfoResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error respawning process with id '%s'", input.ID))}
				}
				return &InfoResponse{Info: newProc.Info(ctx), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func processRegisterSignalTriggerID() cli.Command {
	return cli.Command{
		Name:   "register-signal-trigger-id",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &SignalTriggerIDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))
				}
				return makeOutcomeResponse(errors.Wrapf(proc.RegisterSignalTriggerID(ctx, input.SignalTriggerID), "couldn't register signal trigger with id '%s' on process with id '%s'", input.SignalTriggerID, input.ID))
			})
		},
	}
}

func processTag() cli.Command {
	return cli.Command{
		Name:   "tag",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &TagIDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))
				}
				proc.Tag(input.Tag)
				return makeOutcomeResponse(nil)
			})
		},
	}
}

func processGetTags() cli.Command {
	return cli.Command{
		Name:   "get-tags",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return &TagsResponse{OutcomeResponse: *makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))}
				}
				return &TagsResponse{Tags: proc.GetTags(), OutcomeResponse: *makeOutcomeResponse(nil)}
			})
		},
	}
}

func processResetTags() cli.Command {
	return cli.Command{
		Name:   "reset-tags",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			input := &IDInput{}
			return doPassthroughInputOutput(c, input, func(ctx context.Context, client jasper.RemoteClient) interface{} {
				proc, err := client.Get(ctx, input.ID)
				if err != nil {
					return makeOutcomeResponse(errors.Wrapf(err, "error finding process with id '%s'", input.ID))
				}
				proc.ResetTags()
				return makeOutcomeResponse(nil)
			})
		},
	}
}
