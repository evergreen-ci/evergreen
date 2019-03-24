package jasper

import (
	"context"
	"syscall"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ProcessTrigger describes the way to write cleanup functions for
// processes, which provide ways of adding behavior to processes after
// they complete. A ProcessTrigger can read the fields within
// ProcessInfo.Options but should not mutate them.
type ProcessTrigger func(ProcessInfo)

// ProcessTriggerSequence is simply a convenience type to simplify
// running more than one triggered operation.
type ProcessTriggerSequence []ProcessTrigger

// Run loops over triggers and calls each of them successively.
func (s ProcessTriggerSequence) Run(info ProcessInfo) {
	for _, trigger := range s {
		trigger(info)
	}
}

// SignalTrigger describes the way to write hooks that will execute
// before a process is about to be signaled. It returns a bool
// indicating if the signal should be skipped after execution of the
// trigger. A SignalTrigger can read the fields within ProcessInfo.Options but
// should not mutate them.
type SignalTrigger func(ProcessInfo, syscall.Signal) (skipSignal bool)

// SignalTriggerSequence is a convenience type to simplify running
// more than one signal trigger.
type SignalTriggerSequence []SignalTrigger

// Run loops over signal triggers and calls each of them successively.
// It returns a boolean indicating whether or not the signal should
// be skipped after executing all of the signal triggers.
func (s SignalTriggerSequence) Run(info ProcessInfo, sig syscall.Signal) (skipSignal bool) {
	skipSignal = false
	for _, trigger := range s {
		skipSignal = trigger(info, sig) || skipSignal
	}
	return
}

// SignalTriggerID is the unique representation of a signal trigger.
type SignalTriggerID string

const (
	// CleanTerminationSignalTrigger is the ID for the signal trigger to use for
	// termination of processes with exit code 0.
	CleanTerminationSignalTrigger SignalTriggerID = "clean_terminate"
)

func makeOptionsCloseTrigger() ProcessTrigger {
	return func(info ProcessInfo) {
		if err := info.Options.Close(); err != nil {
			grip.Warning(errors.Wrap(err, "error occurred while closing options"))
		}
	}
}

func makeDefaultTrigger(ctx context.Context, m Manager, opts *CreateOptions, parentID string) ProcessTrigger {
	deadline, hasDeadline := ctx.Deadline()
	timeout := time.Until(deadline)

	return func(info ProcessInfo) {
		switch {
		case info.Timeout:
			var (
				newctx context.Context
				cancel context.CancelFunc
			)

			for _, opt := range opts.OnTimeout {
				if hasDeadline {
					newctx, cancel = context.WithTimeout(context.Background(), timeout)
				} else {
					newctx, cancel = context.WithCancel(ctx)
				}

				p, err := m.CreateProcess(newctx, opt.Copy())
				if err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"trigger": "on-timeout",
						"parent":  parentID,
					}))
					cancel()
					continue
				}
				p.Tag(parentID)
				_ = p.RegisterTrigger(ctx, func(_ ProcessInfo) { cancel() })
			}
		case info.Successful:
			for _, opt := range opts.OnSuccess {
				p, err := m.CreateProcess(ctx, opt.Copy())
				if err != nil {
					grip.Warning(message.WrapError(err, message.Fields{
						"trigger": "on-success",
						"parent":  parentID,
					}))
					continue
				}
				p.Tag(parentID)
			}
		case !info.Successful:
			for _, opt := range opts.OnFailure {
				p, err := m.CreateProcess(ctx, opt.Copy())
				if err != nil {

					grip.Warning(message.WrapError(err, message.Fields{
						"trigger": "on-failure",
						"parent":  parentID,
					}))
					continue
				}
				p.Tag(parentID)
			}
		}
	}
}
