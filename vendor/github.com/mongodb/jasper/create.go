package jasper

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/google/shlex"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// CreateOptions contains options related to starting a process. This includes
// execution configuration, post-execution triggers, and output configuration.
// It is not safe for concurrent access.
type CreateOptions struct {
	Args             []string          `bson:"args" json:"args" yaml:"args"`
	Environment      map[string]string `bson:"env,omitempty" json:"env,omitempty" yaml:"env,omitempty"`
	WorkingDirectory string            `bson:"working_directory,omitempty" json:"working_directory,omitempty" yaml:"working_directory,omitempty"`
	Output           OutputOptions     `bson:"output" json:"output" yaml:"output"`
	OverrideEnviron  bool              `bson:"override_env,omitempty" json:"override_env,omitempty" yaml:"override_env,omitempty"`
	// TimeoutSecs takes precedence over Timeout. On remote interfaces,
	// TimeoutSecs should be set instead of Timeout.
	TimeoutSecs   int              `bson:"timeout_secs,omitempty" json:"timeout_secs,omitempty" yaml:"timeout_secs,omitempty"`
	Timeout       time.Duration    `bson:"timeout" json:"-" yaml:"-"`
	Tags          []string         `bson:"tags" json:"tags,omitempty" yaml:"tags"`
	OnSuccess     []*CreateOptions `bson:"on_success" json:"on_success,omitempty" yaml:"on_success"`
	OnFailure     []*CreateOptions `bson:"on_failure" json:"on_failure,omitempty" yaml:"on_failure"`
	OnTimeout     []*CreateOptions `bson:"on_timeout" json:"on_timeout,omitempty" yaml:"on_timeout"`
	StandardInput io.Reader        `bson:"-" json:"-" yaml:"-"`
	// StandardInputBytes takes precedence over StandardInput. On remote
	// interfaces, StandardInputBytes should be set instead of StandardInput.
	StandardInputBytes []byte `bson:"stdin_bytes" json:"stdin_bytes" yaml:"stdin_bytes"`

	closers []func() error
}

// MakeCreationOptions takes a command string and returns an equivalent
// CreateOptions struct that would spawn a process corresponding to the given
// command string.
func MakeCreationOptions(cmdStr string) (*CreateOptions, error) {
	args, err := shlex.Split(cmdStr)
	if err != nil {
		return nil, errors.Wrap(err, "problem parsing shell command")
	}

	if len(args) == 0 {
		return nil, errors.Errorf("'%s' did not parse to valid args array", cmdStr)
	}

	return &CreateOptions{
		Args: args,
		Output: OutputOptions{
			Output: send.MakeWriterSender(grip.GetSender(), level.Info),
			Error:  send.MakeWriterSender(grip.GetSender(), level.Error),
		},
	}, nil
}

// Validate ensures that CreateOptions is valid for non-remote interfaces.
func (opts *CreateOptions) Validate() error {
	if len(opts.Args) == 0 {
		return errors.New("invalid command, must specify at least one argument")
	}

	if opts.Timeout > 0 && opts.Timeout < time.Second {
		return errors.New("when specifying a timeout, it must be greater than one second")
	}

	if opts.Timeout != 0 && opts.TimeoutSecs != 0 {
		if time.Duration(opts.TimeoutSecs)*time.Second != opts.Timeout {
			return errors.Errorf("cannot specify timeout (nanos) [%s] and timeout_secs [%d]",
				opts.Timeout, opts.Timeout)
		}
	}

	if opts.TimeoutSecs > 0 && opts.Timeout == 0 {
		opts.Timeout = time.Duration(opts.TimeoutSecs) * time.Second
	} else if opts.Timeout != 0 {
		opts.TimeoutSecs = int(opts.Timeout.Seconds())
	}

	if err := opts.Output.Validate(); err != nil {
		return errors.Wrap(err, "cannot create command with invalid output")
	}

	if opts.WorkingDirectory != "" {
		info, err := os.Stat(opts.WorkingDirectory)

		if os.IsNotExist(err) {
			return errors.Errorf("could not use non-extant %s as working directory", opts.WorkingDirectory)
		}

		if !info.IsDir() {
			return errors.Errorf("could not use file as working directory")
		}
	}

	if len(opts.StandardInputBytes) != 0 {
		opts.StandardInput = bytes.NewBuffer(opts.StandardInputBytes)
	}

	return nil
}

func (opts *CreateOptions) hash() hash.Hash {
	hash := sha1.New()

	_, _ = io.WriteString(hash, opts.WorkingDirectory)
	for _, a := range opts.Args {
		_, _ = io.WriteString(hash, a)
	}

	for _, t := range opts.Tags {
		_, _ = io.WriteString(hash, t)
	}

	env := []string{}
	for k, v := range opts.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	sort.Strings(env)
	for _, e := range env {
		_, _ = io.WriteString(hash, e)
	}

	return hash
}

// Resolve creates the command object according to the create options. It
// returns the resolved command and the deadline when the command will be
// terminated by timeout. If there is no deadline, it returns the zero time.
func (opts *CreateOptions) Resolve(ctx context.Context) (*exec.Cmd, time.Time, error) {
	var err error
	if ctx.Err() != nil {
		return nil, time.Time{}, errors.New("cannot resolve command with canceled context")
	}

	if err = opts.Validate(); err != nil {
		return nil, time.Time{}, errors.WithStack(err)
	}

	if opts.WorkingDirectory == "" {
		opts.WorkingDirectory, _ = os.Getwd()
	}

	var env []string
	if !opts.OverrideEnviron {
		env = os.Environ()
	}

	env = append(env, opts.getEnvSlice()...)

	var args []string
	if len(opts.Args) > 1 {
		args = opts.Args[1:]
	}

	var deadline time.Time
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		deadline, _ = ctx.Deadline()
		opts.closers = append(opts.closers, func() error { cancel(); return nil })
	}

	cmd := exec.CommandContext(ctx, opts.Args[0], args...) // nolint
	cmd.Dir = opts.WorkingDirectory

	cmd.Stdout, err = opts.Output.GetOutput()
	if err != nil {
		return nil, time.Time{}, errors.WithStack(err)
	}
	cmd.Stderr, err = opts.Output.GetError()
	if err != nil {
		return nil, time.Time{}, errors.WithStack(err)
	}
	cmd.Env = env

	if opts.StandardInput != nil {
		cmd.Stdin = opts.StandardInput
	}

	// Senders require Close() or else command output is not guaranteed to log.
	opts.closers = append(opts.closers, func() error {
		catcher := grip.NewBasicCatcher()
		if opts.Output.outputSender != nil {
			catcher.Add(opts.Output.outputSender.Close())
		}
		if opts.Output.errorSender != nil {
			catcher.Add(opts.Output.errorSender.Close())
		}
		if opts.Output.outputSender != nil {
			catcher.Add(opts.Output.outputSender.Sender.Close())
		}
		// Since senders are shared, only close error's senders if output hasn't already closed them.
		if opts.Output.errorSender != nil && (opts.Output.SuppressOutput || opts.Output.SendOutputToError) {
			catcher.Add(opts.Output.errorSender.Sender.Close())
		}

		return errors.WithStack(catcher.Resolve())
	})

	return cmd, deadline, nil
}

// getEnvSlice returns the (CreateOptions).Environment as a slice of environment
// variables in the form "key=value".
func (opts *CreateOptions) getEnvSlice() []string {
	env := []string{}
	for k, v := range opts.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// AddEnvVar adds an environment variable to the CreateOptions struct on which
// this method is called. If the Environment map is nil, this method will
// instantiate one.
func (opts *CreateOptions) AddEnvVar(k, v string) {
	if opts.Environment == nil {
		opts.Environment = make(map[string]string)
	}

	opts.Environment[k] = v
}

// Close will execute the closer functions assigned to the CreateOptions. This
// function is often called as a trigger at the end of a process' lifetime in
// Jasper.
func (opts *CreateOptions) Close() error {
	catcher := grip.NewBasicCatcher()
	for _, c := range opts.closers {
		catcher.Add(c())
	}
	return catcher.Resolve()
}

// Copy returns a copy of the options for only the exported fields. Unexported
// fields are cleared.
func (opts *CreateOptions) Copy() *CreateOptions {
	optsCopy := *opts

	if opts.Args != nil {
		optsCopy.Args = make([]string, len(opts.Args))
		_ = copy(optsCopy.Args, opts.Args)
	}

	if opts.Tags != nil {
		optsCopy.Tags = make([]string, len(opts.Tags))
		_ = copy(optsCopy.Tags, opts.Tags)
	}

	if opts.Environment != nil {
		optsCopy.Environment = make(map[string]string)
		for key, val := range opts.Environment {
			optsCopy.Environment[key] = val
		}
	}

	if opts.OnSuccess != nil {
		optsCopy.OnSuccess = make([]*CreateOptions, len(opts.OnSuccess))
		_ = copy(optsCopy.OnSuccess, opts.OnSuccess)
	}

	if opts.OnFailure != nil {
		optsCopy.OnFailure = make([]*CreateOptions, len(opts.OnFailure))
		_ = copy(optsCopy.OnFailure, opts.OnFailure)
	}

	if opts.OnTimeout != nil {
		optsCopy.OnTimeout = make([]*CreateOptions, len(opts.OnTimeout))
		_ = copy(optsCopy.OnTimeout, opts.OnTimeout)
	}

	if opts.StandardInputBytes != nil {
		optsCopy.StandardInputBytes = make([]byte, len(opts.StandardInputBytes))
		_ = copy(optsCopy.StandardInputBytes, opts.StandardInputBytes)
	}

	optsCopy.Output = *opts.Output.Copy()

	optsCopy.closers = nil

	return &optsCopy
}
