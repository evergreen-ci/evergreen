package jasper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/google/shlex"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// Command objects allow a quick and lightweight interface for firing off
// ad-hoc processes for smaller tasks. Command immediately supports features
// such as output and error functionality and remote execution.
type Command struct {
	cmds     [][]string
	opts     *CreateOptions
	priority level.Priority
	id       string
	procIDs  []string

	continueOnError bool
	ignoreError     bool
	prerequisite    func() bool

	makep ProcessConstructor
}

func getRemoteCreateOpt(ctx context.Context, host string, args []string, dir string) (*CreateOptions, error) {
	var remoteCmd string

	if dir != "" {
		remoteCmd = fmt.Sprintf("cd %s && ", dir)
	}

	switch len(args) {
	case 0:
		return nil, errors.New("args invalid")
	case 1:
		remoteCmd += args[0]
	default:
		remoteCmd += strings.Join(args, " ")
	}

	return &CreateOptions{Args: []string{"ssh", host, remoteCmd}}, nil
}

func getLogOutput(out []byte) string {
	return strings.Trim(strings.Replace(string(out), "\n", "\n\t out -> ", -1), "\n\t out->")
}

func splitCmdToArgs(cmd string) []string {
	args, err := shlex.Split(cmd)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{"input": cmd}))
		return nil
	}
	return args
}

// NewCommand returns a blank Command.
// New blank Commands will use basicProcess as their default Process for
// executing sub-commands unless it is changed via ProcConstructor().
func NewCommand() *Command { return &Command{opts: &CreateOptions{}, makep: newBasicProcess} }

// ProcConstructor returns a blank Command that will use the process created
// by the given ProcessConstructor.
func (c *Command) ProcConstructor(processConstructor ProcessConstructor) *Command {
	c.makep = processConstructor
	return c
}

// GetProcIDs returns an array of Process IDs associated with the sub-commands
// being run. This method will return a nil slice until processes have actually
// been created by the Command for execution.
func (c *Command) GetProcIDs() []string { return c.procIDs }

// ApplyFromOpts uses the CreateOptions to configure the Command.
func (c *Command) ApplyFromOpts(opts *CreateOptions) *Command { c.opts = opts; return c }

// String returns a stringified representation.
func (c *Command) String() string { return fmt.Sprintf("id='%s', cmd='%s'", c.id, c.getCmd()) }

// Add adds on a sub-command.
func (c *Command) Add(args []string) *Command { c.cmds = append(c.cmds, args); return c }

// Extend adds on multiple sub-commands.
func (c *Command) Extend(cmds [][]string) *Command { c.cmds = append(c.cmds, cmds...); return c }

// Directory sets the working directory.
func (c *Command) Directory(d string) *Command { c.opts.WorkingDirectory = d; return c }

// Host sets the hostname. A blank hostname implies local execution of the
// command, a non-blank hostname is treated as a remotely executed command.
func (c *Command) Host(h string) *Command { c.opts.Hostname = h; return c }

// Priority sets the logging priority.
func (c *Command) Priority(l level.Priority) *Command { c.priority = l; return c }

// ID sets the ID.
func (c *Command) ID(id string) *Command { c.id = id; return c }

// ContinueOnError sets a flag for determining if the Command should continue
// executing its sub-commands even if one of them errors.
func (c *Command) ContinueOnError(cont bool) *Command { c.continueOnError = cont; return c }

// IgnoreError sets a flag for determining if the Command should return a nil
// error despite errors in its sub-command executions.
func (c *Command) IgnoreError(ignore bool) *Command { c.ignoreError = ignore; return c }

// Environment replaces the current environment map with the given environment
// map.
func (c *Command) Environment(e map[string]string) *Command { c.opts.Environment = e; return c }

// AddEnv adds a key value pair of environment variable to value into the
// Command's environment variable map.
func (c *Command) AddEnv(k, v string) *Command { c.setupEnv(); c.opts.Environment[k] = v; return c }

// Prerequisite sets a function on the Command such that the Command will only
// execute if the function returns true.
func (c *Command) Prerequisite(chk func() bool) *Command { c.prerequisite = chk; return c }

// Append takes a series of strings and splits them into sub-commands and adds
// them to the Command.
func (c *Command) Append(cmds ...string) *Command {
	for _, cmd := range cmds {
		c.cmds = append(c.cmds, splitCmdToArgs(cmd))
	}
	return c
}

func (c *Command) setupEnv() {
	if c.opts.Environment == nil {
		c.opts.Environment = map[string]string{}
	}
}

// Run starts and then waits on the Command's execution.
func (c *Command) Run(ctx context.Context) error {
	if c.prerequisite != nil && !c.prerequisite() {
		grip.Debug(message.Fields{
			"op":  "noop after prerequisite returned false",
			"id":  c.id,
			"cmd": c.String(),
		})
		return nil
	}

	c.finalizeWriters()
	catcher := grip.NewBasicCatcher()

	var opts []*CreateOptions
	opts, err := c.getCreateOpts(ctx)
	if err != nil {
		catcher.Add(err)
		catcher.Add(c.Close())
		return catcher.Resolve()
	}

	for idx, opt := range opts {
		if err := ctx.Err(); err != nil {
			catcher.Add(errors.Wrap(err, "operation canceled"))
			catcher.Add(c.Close())
			return catcher.Resolve()
		}

		err := c.exec(ctx, opt, idx)
		if !c.ignoreError {
			catcher.Add(err)
		}

		if err != nil && !c.continueOnError {
			catcher.Add(c.Close())
			return catcher.Resolve()
		}
	}

	catcher.Add(c.Close())
	return catcher.Resolve()
}

// RunParallel is the same as Run(), but will run all sub-commands in parallel.
// Use of this function effectively ignores the the ContinueOnError flag.
func (c *Command) RunParallel(ctx context.Context) error {
	// Avoid paying the copy-costs in between command structs by doing the work
	// before executing the commands.
	parallelCmds := make([]Command, len(c.cmds))

	for idx, cmd := range c.cmds {
		splitCmd := *c
		optsCopy := *(c.opts)
		splitCmd.opts = &optsCopy
		splitCmd.opts.closers = []func() error{}
		splitCmd.cmds = [][]string{cmd}
		parallelCmds[idx] = splitCmd
	}

	errs := make(chan error, len(c.cmds))
	for _, parallelCmd := range parallelCmds {
		go func(innerCmd Command) {
			defer func() {
				err := recovery.HandlePanicWithError(recover(), nil, "parallel command encountered error")
				if err != nil {
					errs <- err
				}
			}()
			errs <- innerCmd.Run(ctx)
		}(parallelCmd)
	}

	catcher := grip.NewBasicCatcher()
	for i := 0; i < len(c.cmds); i++ {
		select {
		case err := <-errs:
			if !c.ignoreError {
				catcher.Add(err)
			}
		case <-ctx.Done():
			catcher.Add(c.Close())
			catcherErr := catcher.Resolve()
			if catcherErr != nil {
				return errors.Wrapf(ctx.Err(), catcherErr.Error())
			}
			return ctx.Err()
		}
	}

	catcher.Add(c.Close())
	return catcher.Resolve()
}

// Close closes this command and its resources.
func (c *Command) Close() error {
	return c.opts.Close()
}

// SetErrorSender sets a Sender to be used by this Command for its output to
// stderr.
func (c *Command) SetErrorSender(l level.Priority, s send.Sender) *Command {
	writer := send.MakeWriterSender(s, l)
	c.opts.closers = append(c.opts.closers, writer.Close)
	c.opts.Output.Error = writer
	return c
}

// SetOutputSender sets a Sender to be used by this Command for its output to
// stdout.
func (c *Command) SetOutputSender(l level.Priority, s send.Sender) *Command {
	writer := send.MakeWriterSender(s, l)
	c.opts.closers = append(c.opts.closers, writer.Close)
	c.opts.Output.Output = writer
	return c
}

// SetCombinedSender is the combination of SetErrorSender() and
// SetOutputSender().
func (c *Command) SetCombinedSender(l level.Priority, s send.Sender) *Command {
	writer := send.MakeWriterSender(s, l)
	c.opts.closers = append(c.opts.closers, writer.Close)
	c.opts.Output.Error = writer
	c.opts.Output.Output = writer
	return c
}

// SetErrorWriter sets a Writer to be used by this Command for its output to
// stderr.
func (c *Command) SetErrorWriter(writer io.WriteCloser) *Command {
	c.opts.closers = append(c.opts.closers, writer.Close)
	c.opts.Output.Error = writer
	return c
}

// SetOutputWriter sets a Writer to be used by this Command for its output to
// stdout.
func (c *Command) SetOutputWriter(writer io.WriteCloser) *Command {
	c.opts.closers = append(c.opts.closers, writer.Close)
	c.opts.Output.Output = writer
	return c
}

// SetCombinedWriter is the combination of SetErrorWriter() and
// SetOutputWriter().
func (c *Command) SetCombinedWriter(writer io.WriteCloser) *Command {
	c.opts.closers = append(c.opts.closers, writer.Close)
	c.opts.Output.Error = writer
	c.opts.Output.Output = writer
	return c
}

func (c *Command) finalizeWriters() {
	if c.opts.Output.Output == nil {
		c.opts.Output.Output = ioutil.Discard
	}

	if c.opts.Output.Error == nil {
		c.opts.Output.Error = ioutil.Discard
	}
}

func (c *Command) getEnv() []string {
	out := []string{}
	for k, v := range c.opts.Environment {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}

func (c *Command) getCmd() string {
	env := strings.Join(c.getEnv(), " ")
	out := []string{}
	for _, cmd := range c.cmds {
		out = append(out, fmt.Sprintf("%s '%s';\n", env, strings.Join(cmd, " ")))
	}
	return strings.Join(out, "")
}

func getCreateOpt(ctx context.Context, args []string, dir string, env map[string]string) (*CreateOptions, error) {
	var opts *CreateOptions
	switch len(args) {
	case 0:
		return nil, errors.New("args invalid")
	case 1:
		if strings.Contains(args[0], " \"'") {
			spl, err := shlex.Split(args[0])
			if err != nil {
				return nil, errors.Wrap(err, "problem splitting argstring")
			}
			return getCreateOpt(ctx, spl, dir, env)
		}
		opts = &CreateOptions{Args: args}
	default:
		opts = &CreateOptions{Args: args}
	}
	opts.WorkingDirectory = dir

	for k, v := range env {
		opts.Environment[k] = v
	}

	return opts, nil
}

func (c *Command) getCreateOpts(ctx context.Context) ([]*CreateOptions, error) {
	out := []*CreateOptions{}
	catcher := grip.NewBasicCatcher()
	if c.opts.Hostname != "" {
		for _, args := range c.cmds {
			cmd, err := getRemoteCreateOpt(ctx, c.opts.Hostname, args, c.opts.WorkingDirectory)
			if err != nil {
				catcher.Add(err)
				continue
			}

			out = append(out, cmd)
		}
	} else {
		for _, args := range c.cmds {
			cmd, err := getCreateOpt(ctx, args, c.opts.WorkingDirectory, c.opts.Environment)
			if err != nil {
				catcher.Add(err)
				continue
			}

			out = append(out, cmd)
		}
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return out, nil
}

func (c *Command) exec(ctx context.Context, opts *CreateOptions, idx int) error {
	msg := message.Fields{
		"id":  c.id,
		"cmd": strings.Join(opts.Args, " "),
		"idx": idx,
		"len": len(c.cmds),
	}

	var err error
	var newProc Process
	if c.opts.Output.Output == nil {
		var out bytes.Buffer
		opts.Output.Output = &out
		opts.Output.Error = &out
		newProc, err = c.makep(ctx, opts)
		if err != nil {
			return errors.Wrapf(err, "problem starting command")
		}

		c.procIDs = append(c.procIDs, newProc.ID())
		_, err = newProc.Wait(ctx)
		msg["out"] = getLogOutput(out.Bytes())
		msg["err"] = err
	} else {
		opts.Output.Error = c.opts.Output.Error
		opts.Output.Output = c.opts.Output.Output
		newProc, err = c.makep(ctx, opts)
		if err != nil {
			return errors.Wrapf(err, "problem starting command")
		}

		c.procIDs = append(c.procIDs, newProc.ID())
		_, err = newProc.Wait(ctx)
		msg["err"] = err
	}

	grip.Log(c.priority, msg)
	return err
}

// RunCommand runs the Command given the configuration of arguments.
func RunCommand(ctx context.Context, id string, pri level.Priority, args []string, dir string, env map[string]string) error {
	return NewCommand().ID(id).Priority(pri).Add(args).Directory(dir).Environment(env).Run(ctx)
}

// RunRemoteCommand runs the Command remotely given the configuration of arguments.
func RunRemoteCommand(ctx context.Context, id string, pri level.Priority, host string, args []string, dir string) error {
	return NewCommand().ID(id).Priority(pri).Host(host).Add(args).Directory(dir).Run(ctx)
}

// RunCommandGroupContinueOnError runs the group of sub-commands given the
// configuration of arguments, continuing execution despite any errors.
func RunCommandGroupContinueOnError(ctx context.Context, id string, pri level.Priority, cmds [][]string, dir string, env map[string]string) error {
	return NewCommand().ID(id).Priority(pri).Extend(cmds).Directory(dir).Environment(env).ContinueOnError(true).Run(ctx)
}

// RunRemoteCommandGroupContinueOnError runs the group of sub-commands remotely
// given the configuration of arguments, continuing execution despite any
// errors.
func RunRemoteCommandGroupContinueOnError(ctx context.Context, id string, pri level.Priority, host string, cmds [][]string, dir string) error {
	return NewCommand().ID(id).Priority(pri).Host(host).Extend(cmds).Directory(dir).ContinueOnError(true).Run(ctx)
}

// RunCommandGroup runs the group of sub-commands given the configuration of
// arguments.
func RunCommandGroup(ctx context.Context, id string, pri level.Priority, cmds [][]string, dir string, env map[string]string) error {
	return NewCommand().ID(id).Priority(pri).Extend(cmds).Directory(dir).Environment(env).Run(ctx)
}

// RunRemoteCommandGroup runs the group of sub-commands remotely given the
// configuration of arguments.
func RunRemoteCommandGroup(ctx context.Context, id string, pri level.Priority, host string, cmds [][]string, dir string) error {
	return NewCommand().ID(id).Priority(pri).Host(host).Extend(cmds).Directory(dir).Run(ctx)
}

// RunParallelCommandGroup runs the group of sub-commands in
// parallel given the configuration of arguments.
func RunParallelCommandGroup(ctx context.Context, id string, pri level.Priority, cmds [][]string, dir string, env map[string]string) error {
	return NewCommand().ID(id).Priority(pri).Extend(cmds).Directory(dir).Environment(env).RunParallel(ctx)
}

// RunParallelRemoteCommandGroup runs the group of sub-commands
// remotely in parallel given the configuration of arguments.
func RunParallelRemoteCommandGroup(ctx context.Context, id string, pri level.Priority, host string, cmds [][]string, dir string) error {
	return NewCommand().ID(id).Priority(pri).Host(host).Extend(cmds).Directory(dir).RunParallel(ctx)
}
