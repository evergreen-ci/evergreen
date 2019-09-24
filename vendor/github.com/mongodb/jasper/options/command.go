package options

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

// Command represents jasper.Command options that are configurable by the
// user.
type Command struct {
	ID              string         `json:"id,omitempty"`
	Commands        [][]string     `json:"commands"`
	Process         Create         `json:"proc_opts,omitempty"`
	Remote          *Remote        `json:"remote_options,omitempty"`
	ContinueOnError bool           `json:"continue_on_error,omitempty"`
	IgnoreError     bool           `json:"ignore_error,omitempty"`
	Priority        level.Priority `json:"priority,omitempty"`
	RunBackground   bool           `json:"run_background,omitempty"`
	Sudo            bool           `json:"sudo,omitempty"`
	SudoUser        string         `json:"sudo_user,omitempty"`
	Prerequisite    func() bool    `json:"-"`
}

// Validate ensures that the options passed to the command are valid.
func (opts *Command) Validate() error {
	catcher := grip.NewBasicCatcher()
	// The semantics of options.Create expects Args to be non-empty, but Command
	// ignores these args.
	if len(opts.Process.Args) == 0 {
		opts.Process.Args = []string{""}
	}
	catcher.Add(opts.Process.Validate())
	if opts.Priority != 0 && !level.IsValidPriority(opts.Priority) {
		catcher.Add(errors.New("priority is not in the valid range of values"))
	}

	if len(opts.Commands) == 0 {
		catcher.Add(errors.New("must specify at least one command"))
	}
	return catcher.Resolve()
}
