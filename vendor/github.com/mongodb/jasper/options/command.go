package options

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

// Command represents jasper.Command options that are configurable by the
// user.
type Command struct {
	ID              string          `json:"id,omitempty"`
	Commands        [][]string      `json:"commands"`
	Process         Create          `json:"proc_opts,omitempty"`
	Remote          *Remote         `json:"remote_options,omitempty"`
	ContinueOnError bool            `json:"continue_on_error,omitempty"`
	IgnoreError     bool            `json:"ignore_error,omitempty"`
	Priority        level.Priority  `json:"priority,omitempty"`
	RunBackground   bool            `json:"run_background,omitempty"`
	Sudo            bool            `json:"sudo,omitempty"`
	SudoUser        string          `json:"sudo_user,omitempty"`
	Prerequisite    func() bool     `json:"-"`
	PostHook        CommandPostHook `json:"-"`
	PreHook         CommandPreHook  `json:"-"`
}

// Validate ensures that the options passed to the command are valid.
func (opts *Command) Validate() error {
	catcher := grip.NewBasicCatcher()
	// The semantics of a valid options.Create expects Args to be non-empty, but
	// Command ignores these args, so we insert a dummy argument.
	if len(opts.Process.Args) == 0 {
		opts.Process.Args = []string{""}
	}
	catcher.Add(opts.Process.Validate())
	catcher.NewWhen(opts.Priority != 0 && !opts.Priority.IsValid(), "priority is not in the valid range of values")
	catcher.NewWhen(len(opts.Commands) == 0, "must specify at least one command")
	return catcher.Resolve()
}

// CommandPreHook describes a common function type to run before
// sub-commands in a command object and can modify the state of the
// command.
type CommandPreHook func(*Command, *Create)

// NewLoggingPreHook provides a logging message for debugging purposes
// that prints information from the creation options.
func NewLoggingPreHook(logger grip.Journaler, lp level.Priority) CommandPreHook {
	return func(cmd *Command, opt *Create) {
		logger.Log(lp, message.Fields{
			"id":     cmd.ID,
			"dir":    opt.WorkingDirectory,
			"cmd":    opt.Args,
			"tags":   opt.Tags,
			"remote": opt.Remote != nil,
		})
	}
}

// NewDefaultLoggingPreHook uses the global grip logger to log a debug
// message at the specified priority level.
func NewDefaultLoggingPreHook(lp level.Priority) CommandPreHook {
	return NewLoggingPreHook(grip.GetDefaultJournaler(), lp)
}

// NewLoggingPreHookFromSender produces a logging prehook that wraps a sender.
func NewLoggingPreHookFromSender(sender send.Sender, lp level.Priority) CommandPreHook {
	return NewLoggingPreHook(logging.MakeGrip(sender), lp)
}

// MergePreHooks produces a single PreHook function that runs all
// defined prehooks.
func MergePreHooks(fns ...CommandPreHook) CommandPreHook {
	return func(cmd *Command, opt *Create) {
		for _, fn := range fns {
			fn(cmd, opt)
		}
	}
}

// CommandPostHook describes a common function type to run after the
// command completes and processes its error, potentially overriding
// the output of the command.
type CommandPostHook func(error) error

// MergePostHooks runs all post hooks, collecting
// the errors and merging them.
func MergePostHooks(fns ...CommandPostHook) CommandPostHook {
	return func(err error) error {
		catcher := grip.NewBasicCatcher()
		for _, fn := range fns {
			catcher.Add(fn(err))
		}
		return catcher.Resolve()
	}
}

// MergeAbortingPostHooks produces a post hook that returns the first
// error returned by one of it's argument functions, and returns nil
// if no function returns an error.
func MergeAbortingPostHooks(fns ...CommandPostHook) CommandPostHook {
	return func(err error) error {
		for _, fn := range fns {
			if innerErr := fn(err); innerErr != nil {
				return innerErr
			}
		}
		return nil
	}
}

// MergeAbortingPassthroughPostHooks produces a post hook that returns
// the error of the first post hook passed that does not return
// nil. If none of the passed functions return an error, the hook
// returns the original error.
func MergeAbortingPassthroughPostHooks(fns ...CommandPostHook) CommandPostHook {
	return func(err error) error {
		for _, fn := range fns {
			if innerErr := fn(err); innerErr != nil {
				return innerErr
			}
		}
		if err != nil {
			return err
		}
		return nil
	}
}
