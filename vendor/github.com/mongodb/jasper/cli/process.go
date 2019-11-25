package cli

import (
	"context"
	"syscall"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Constants representing the Jasper Process interface as CLI commands.
const (
	ProcessCommand                 = "process"
	InfoCommand                    = "info"
	CompleteCommand                = "complete"
	RegisterSignalTriggerIDCommand = "register-signal-trigger-id"
	RespawnCommand                 = "respawn"
	RunningCommand                 = "running"
	SignalCommand                  = "signal"
	TagCommand                     = "tag"
	GetTagsCommand                 = "get-tags"
	ResetTagsCommand               = "reset-tags"
	WaitCommand                    = "wait"
)

// Process creates a cli.Command that interfaces with a Jasper process. Due to
// it being a remote process, there is no CLI equivalent of of RegisterTrigger
// or RegisterSignalTrigger.
func Process() cli.Command {
	return cli.Command{
		Name: ProcessCommand,
		Subcommands: []cli.Command{
			processInfo(),
			processRunning(),
			processComplete(),
			processTag(),
			processGetTags(),
			processResetTags(),
			processRespawn(),
			processRegisterSignalTriggerID(),
			processSignal(),
			processWait(),
		},
	}
}

func processInfo() cli.Command {
	return cli.Command{
		Name:   InfoCommand,
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
		Name:   RunningCommand,
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
		Name:   CompleteCommand,
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
		Name:   SignalCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
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
		Name:   WaitCommand,
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
		Name:   RespawnCommand,
		Flags:  clientFlags(),
		Before: clientBefore(),
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
		Name:   RegisterSignalTriggerIDCommand,
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
		Name:   TagCommand,
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
		Name:   GetTagsCommand,
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
		Name:   ResetTagsCommand,
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
