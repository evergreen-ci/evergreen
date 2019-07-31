package jasper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/google/shlex"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// Command objects allow a quick and lightweight interface for firing off
// ad-hoc processes for smaller tasks. Command immediately supports features
// such as output and error functionality and remote execution. Command methods
// are not thread-safe.
type Command struct {
	continueOnError bool
	ignoreError     bool
	opts            *CreateOptions
	prerequisite    func() bool
	priority        level.Priority
	runBackground   bool
	sudo            bool
	sudoUser        string

	cmds   [][]string
	id     string
	remote RemoteOptions
	makep  ProcessConstructor

	procs []Process
}

func (c *Command) sudoCmd() []string {
	sudoCmd := []string{}
	if c.sudo {
		sudoCmd = append(sudoCmd, "sudo")
		if c.sudoUser != "" {
			sudoCmd = append(sudoCmd, "-u", c.sudoUser)
		}
	}
	return sudoCmd
}

func (c *Command) getRemoteCreateOpt(ctx context.Context, args []string) (*CreateOptions, error) {
	opts := c.opts.Copy()
	opts.WorkingDirectory = ""
	opts.Environment = nil

	var remoteCmd string

	if c.opts.WorkingDirectory != "" {
		remoteCmd += fmt.Sprintf("cd '%s' && ", c.opts.WorkingDirectory)
	}

	if env := c.opts.getEnvSlice(); len(env) != 0 {
		remoteCmd += strings.Join(env, " ") + " "
	}

	if c.sudo {
		args = append(c.sudoCmd(), args...)
	}

	switch len(args) {
	case 0:
		return nil, errors.New("cannot have empty args")
	case 1:
		remoteCmd += args[0]
	default:
		remoteCmd += strings.Join(args, " ")
	}

	opts.Args = append(append([]string{"ssh"}, c.remote.Args...), c.remote.hostString(), remoteCmd)
	return opts, nil
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
func NewCommand() *Command {
	return &Command{opts: &CreateOptions{}, makep: newBasicProcess}
}

// ProcConstructor returns a blank Command that will use the process created
// by the given ProcessConstructor.
func (c *Command) ProcConstructor(processConstructor ProcessConstructor) *Command {
	c.makep = processConstructor
	return c
}

// GetProcIDs returns an array of Process IDs associated with the sub-commands
// being run. This method will return a nil slice until processes have actually
// been created by the Command for execution.
func (c *Command) GetProcIDs() []string {
	ids := []string{}
	for _, proc := range c.procs {
		ids = append(ids, proc.ID())
	}
	return ids
}

// ApplyFromOpts uses the CreateOptions to configure the Command. If this is a
// remote command (i.e. host has been set), the WorkingDirectory and Environment
// will apply to the command being run on remote.
// If Args is set on the CreateOptions, it will be ignored; the Args can be
// added using Add, Append, AppendArgs, or Extend.
// This overwrites options that were previously set in the following functions:
// AddEnv, Environment, RedirectErrorToOutput, RedirectOutputToError,
// SetCombinedSender, SetErrorSender, SetErrorWriter, SetOutputOptions,
// SetOutputSender, SetOutputWriter, SuppressStandardError, and
// SuppressStandardOutput.
func (c *Command) ApplyFromOpts(opts *CreateOptions) *Command { c.opts = opts; return c }

// SetOutputOptions sets the output options for a command.
func (c *Command) SetOutputOptions(opts OutputOptions) *Command { c.opts.Output = opts; return c }

// String returns a stringified representation.
func (c *Command) String() string {
	return fmt.Sprintf("id='%s', remote='%s', cmd='%s'", c.id, c.remote.hostString(), c.getCmd())
}

// Export returns all of the CreateOptions that will be used to spawn the
// processes that run all subcommands.
func (c *Command) Export(ctx context.Context) ([]*CreateOptions, error) {
	opts, err := c.getCreateOpts(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting create options")
	}
	return opts, nil
}

// Directory sets the working directory. If this is a remote command, it sets
// the working directory of the command being run remotely.
func (c *Command) Directory(d string) *Command { c.opts.WorkingDirectory = d; return c }

// Host sets the hostname. A blank hostname implies local execution of the
// command, a non-blank hostname is treated as a remotely executed command.
func (c *Command) Host(h string) *Command { c.remote.Host = h; return c }

// User sets the username for remote operations. Host name must be set
// to execute as a remote command.
func (c *Command) User(u string) *Command { c.remote.User = u; return c }

// SetRemoteArgs sets the arguments, if any, that are passed to the
// underlying ssh command, for remote commands.
func (c *Command) SetRemoteArgs(args []string) *Command { c.remote.Args = args; return c }

// ExtendRemoteArgs allows you to add arguments, when needed, to the
// underlying ssh command, for remote commands.
func (c *Command) ExtendRemoteArgs(args ...string) *Command {
	c.remote.Args = append(c.remote.Args, args...)
	return c
}

// Priority sets the logging priority.
func (c *Command) Priority(l level.Priority) *Command { c.priority = l; return c }

// ID sets the ID of the Command, which is independent of the IDs of the
// subcommands that are executed.
func (c *Command) ID(id string) *Command { c.id = id; return c }

// SetTags overrides any existing tags for a process with the
// specified list. Tags are used to filter process with the manager.
func (c *Command) SetTags(tags []string) *Command { c.opts.Tags = tags; return c }

// AppendTags adds the specified tags to the existing tag slice. Tags
// are used to filter process with the manager.
func (c *Command) AppendTags(t ...string) *Command { c.opts.Tags = append(c.opts.Tags, t...); return c }

// ExtendTags adds all tags in the specified slice to the tags will be
// added to the process after creation. Tags are used to filter
// process with the manager.
func (c *Command) ExtendTags(t []string) *Command { c.opts.Tags = append(c.opts.Tags, t...); return c }

// Background allows you to set the command to run in the background
// when you call Run(), the command will begin executing but will not
// complete when run returns.
func (c *Command) Background(runBackground bool) *Command { c.runBackground = runBackground; return c }

// ContinueOnError sets a flag for determining if the Command should continue
// executing its sub-commands even if one of them errors.
func (c *Command) ContinueOnError(cont bool) *Command { c.continueOnError = cont; return c }

// IgnoreError sets a flag for determining if the Command should return a nil
// error despite errors in its sub-command executions.
func (c *Command) IgnoreError(ignore bool) *Command { c.ignoreError = ignore; return c }

// SuppressStandardError sets a flag for determining if the Command should
// discard all standard error content.
func (c *Command) SuppressStandardError(v bool) *Command { c.opts.Output.SuppressError = v; return c }

// SetLoggers sets the logging output on this command to the specified
// slice. This removes any loggers previously configured.
func (c *Command) SetLoggers(l []Logger) *Command { c.opts.Output.Loggers = l; return c }

// AppendLoggers adds one or more loggers to the existing configured
// loggers in the command.
func (c *Command) AppendLoggers(l ...Logger) *Command {
	c.opts.Output.Loggers = append(c.opts.Output.Loggers, l...)
	return c
}

// ExtendLoggers takes the existing slice of loggers and adds that to any
// existing configuration.
func (c *Command) ExtendLoggers(l []Logger) *Command {
	c.opts.Output.Loggers = append(c.opts.Output.Loggers, l...)
	return c
}

// SuppressStandardOutput sets a flag for determining if the Command should
// discard all standard output content.
func (c *Command) SuppressStandardOutput(v bool) *Command { c.opts.Output.SuppressOutput = v; return c }

// RedirectOutputToError sets a flag for determining if the Command should send
// all standard output content to standard error.
func (c *Command) RedirectOutputToError(v bool) *Command {
	c.opts.Output.SendOutputToError = v
	return c
}

// RedirectErrorToOutput sets a flag for determining if the Command should send
// all standard error content to standard output.
func (c *Command) RedirectErrorToOutput(v bool) *Command {
	c.opts.Output.SendOutputToError = v
	return c
}

// Environment replaces the current environment map with the given environment
// map. If this is a remote command, it sets the environment of the command
// being run remotely.
func (c *Command) Environment(e map[string]string) *Command { c.opts.Environment = e; return c }

// AddEnv adds a key value pair of environment variable to value into the
// Command's environment variable map. If this is a remote command, it sets the
// environment of the command being run remotely.
func (c *Command) AddEnv(k, v string) *Command { c.setupEnv(); c.opts.Environment[k] = v; return c }

// Prerequisite sets a function on the Command such that the Command will only
// execute if the function returns true.
func (c *Command) Prerequisite(chk func() bool) *Command { c.prerequisite = chk; return c }

// Add adds on a sub-command.
func (c *Command) Add(args []string) *Command { c.cmds = append(c.cmds, args); return c }

// Sudo runs each command with superuser privileges with the default target
// user. This will cause the commands to fail if the commands are executed in
// Windows. If this is a remote command, the command being run remotely uses
// superuser privileges.
func (c *Command) Sudo(sudo bool) *Command { c.sudo = sudo; return c }

// SudoAs runs each command with sudo but allows each command to be run as a
// user other than the default target user (usually root). This will cause the
// commands to fail if the commands are executed in Windows. If this is a remote
// command, the command being run remotely uses superuse privileges.
func (c *Command) SudoAs(user string) *Command { c.sudo = true; c.sudoUser = user; return c }

// Extend adds on multiple sub-commands.
func (c *Command) Extend(cmds [][]string) *Command { c.cmds = append(c.cmds, cmds...); return c }

// Append takes a series of strings and splits them into sub-commands and adds
// them to the Command.
func (c *Command) Append(cmds ...string) *Command {
	for _, cmd := range cmds {
		c.cmds = append(c.cmds, splitCmdToArgs(cmd))
	}
	return c
}

// ShellScript adds an operation to the command that runs a shell script, using
// the shell's "-c" option).
func (c *Command) ShellScript(shell, script string) *Command {
	c.cmds = append(c.cmds, []string{shell, "-c", script})
	return c
}

// Bash adds a script using "bash -c", as syntactic sugar for the ShellScript2
// method.
func (c *Command) Bash(script string) *Command { return c.ShellScript("bash", script) }

// Sh adds a script using "sh -c", as syntactic sugar for the ShellScript
// method.
func (c *Command) Sh(script string) *Command { return c.ShellScript("sh", script) }

// AppendArgs is the variadic equivalent of Add, which adds a command
// in the form of arguments.
func (c *Command) AppendArgs(args ...string) *Command { return c.Add(args) }

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
// Use of this function effectively ignores the ContinueOnError flag.
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
		splitCmd.procs = []Process{}
		parallelCmds[idx] = splitCmd
	}

	type cmdResult struct {
		procs []Process
		err   error
	}
	cmdResults := make(chan cmdResult, len(c.cmds))
	for _, parallelCmd := range parallelCmds {
		go func(innerCmd Command) {
			defer func() {
				err := recovery.HandlePanicWithError(recover(), nil, "parallel command encountered error")
				if err != nil {
					cmdResults <- cmdResult{err: err}
				}
			}()
			err := innerCmd.Run(ctx)
			select {
			case cmdResults <- cmdResult{procs: innerCmd.procs, err: err}:
			case <-ctx.Done():
			}
		}(parallelCmd)
	}

	catcher := grip.NewBasicCatcher()
	for i := 0; i < len(c.cmds); i++ {
		select {
		case cmdRes := <-cmdResults:
			if !c.ignoreError {
				catcher.Add(cmdRes.err)
			}
			c.procs = append(c.procs, cmdRes.procs...)
		case <-ctx.Done():
			c.procs = []Process{}
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

func (c *Command) SetInput(r io.Reader) *Command {
	c.opts.StandardInput = r
	return c
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

// EnqueueForeground adds separate jobs to the queue for every operation
// captured in the command. These operations will execute in
// parallel. The output of the commands are logged, using the default
// grip sender in the foreground.
func (c *Command) EnqueueForeground(ctx context.Context, q amboy.Queue) error {
	jobs, err := c.JobsForeground(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewBasicCatcher()
	for _, j := range jobs {
		catcher.Add(q.Put(ctx, j))
	}

	return catcher.Resolve()
}

// Enqueue adds separate jobs to the queue for every operation
// captured in the command. These operations will execute in
// parallel. The output of the operations is captured in the body of
// the job.
func (c *Command) Enqueue(ctx context.Context, q amboy.Queue) error {
	jobs, err := c.Jobs(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewBasicCatcher()
	for _, j := range jobs {
		catcher.Add(q.Put(ctx, j))
	}

	return catcher.Resolve()
}

// JobsForeground returns a slice of jobs for every operation
// captured in the command. The output of the commands are logged,
// using the default grip sender in the foreground.
func (c *Command) JobsForeground(ctx context.Context) ([]amboy.Job, error) {
	opts, err := c.getCreateOpts(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out := make([]amboy.Job, len(opts))
	for idx := range opts {
		out[idx] = NewJobForeground(c.makep, opts[idx])
	}
	return out, nil
}

// Jobs returns a slice of jobs for every operation in the
// command. The output of the commands are captured in the body of the
// job.
func (c *Command) Jobs(ctx context.Context) ([]amboy.Job, error) {
	opts, err := c.getCreateOpts(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out := make([]amboy.Job, len(opts))
	for idx := range opts {
		out[idx] = NewJobOptions(c.makep, opts[idx])
	}
	return out, nil
}

func (c *Command) finalizeWriters() {
	if c.opts.Output.Output == nil {
		c.opts.Output.Output = ioutil.Discard
	}

	if c.opts.Output.Error == nil {
		c.opts.Output.Error = ioutil.Discard
	}
}

func (c *Command) getCmd() string {
	env := strings.Join(c.opts.getEnvSlice(), " ")
	out := []string{}
	for _, cmd := range c.cmds {
		if c.sudo {
			cmd = append(c.sudoCmd(), cmd...)
		}
		var formattedCmd string
		if len(env) != 0 {
			formattedCmd += fmt.Sprintf("%s ", env)
		}
		formattedCmd += strings.Join(cmd, " ")
		out = append(out, formattedCmd)
	}
	return strings.Join(out, "\n")
}

func (c *Command) getCreateOpt(ctx context.Context, args []string) (*CreateOptions, error) {
	opts := c.opts.Copy()
	switch len(args) {
	case 0:
		return nil, errors.New("cannot have empty args")
	case 1:
		if strings.ContainsAny(args[0], " \"'") {
			spl, err := shlex.Split(args[0])
			if err != nil {
				return nil, errors.Wrap(err, "problem splitting argstring")
			}
			return c.getCreateOpt(ctx, spl)
		}
		opts.Args = args
	default:
		opts.Args = args
	}

	if c.sudo {
		opts.Args = append(c.sudoCmd(), opts.Args...)
	}

	return opts, nil
}

func (c *Command) getCreateOpts(ctx context.Context) ([]*CreateOptions, error) {
	out := []*CreateOptions{}
	catcher := grip.NewBasicCatcher()
	if c.remote.Host != "" {
		for _, args := range c.cmds {
			cmd, err := c.getRemoteCreateOpt(ctx, args)
			if err != nil {
				catcher.Add(err)
				continue
			}

			out = append(out, cmd)
		}
	} else {
		for _, args := range c.cmds {
			cmd, err := c.getCreateOpt(ctx, args)
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
		"id":   c.id,
		"cmd":  strings.Join(opts.Args, " "),
		"idx":  idx,
		"len":  len(c.cmds),
		"bkg":  c.runBackground,
		"tags": c.opts.Tags,
	}

	var err error
	var out bytes.Buffer
	addOutOp := func(msg message.Fields) message.Fields { return msg }
	if opts.Output.Output == nil || opts.Output.Error == nil {
		if opts.Output.Output == nil {
			opts.Output.Output = &out
		}
		if opts.Output.Error == nil {
			opts.Output.Error = &out
		}
		addOutOp = func(msg message.Fields) message.Fields {
			msg["out"] = out.String()
			return msg
		}
	}

	proc, err := c.makep(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "problem starting command")
	}
	c.procs = append(c.procs, proc)

	if !c.runBackground {
		waitCatcher := grip.NewBasicCatcher()
		for _, proc := range c.procs {
			_, err = proc.Wait(ctx)
			waitCatcher.Add(errors.Wrapf(err, "error waiting on process '%s'", proc.ID()))
		}
		msg["err"] = waitCatcher.Resolve()
	}
	grip.Log(c.priority, addOutOp(msg))

	return errors.WithStack(err)
}

// Wait returns the exit code and error waiting for the underlying process to
// complete.
// For commands run with RunParallel, Wait only returns a zero exit code if all
// the underlying processes return exit code zero; otherwise, it returns a
// non-zero exit code. Similarly, it will return a non-nil error if any of the
// underlying processes encounter an error while waiting.
func (c *Command) Wait(ctx context.Context) (int, error) {
	if len(c.procs) == 0 {
		return 0, errors.New("cannot call wait on a command if no processes have started yet")
	}

	for _, proc := range c.procs {
		exitCode, err := proc.Wait(ctx)
		if err != nil || exitCode != 0 {
			return exitCode, errors.Wrapf(err, "error waiting on process '%s'", proc.ID())
		}
	}

	return 0, nil
}

// BuildCommand builds the Command given the configuration of arguments.
func BuildCommand(id string, pri level.Priority, args []string, dir string, env map[string]string) *Command {
	return NewCommand().ID(id).Priority(pri).Add(args).Directory(dir).Environment(env)
}

// BuildRemoteCommand builds the Command remotely given the configuration of arguments.
func BuildRemoteCommand(id string, pri level.Priority, host string, args []string, dir string, env map[string]string) *Command {
	return NewCommand().ID(id).Priority(pri).Host(host).Add(args).Directory(dir).Environment(env)
}

// BuildCommandGroupContinueOnError runs the group of sub-commands given the
// configuration of arguments, continuing execution despite any errors.
func BuildCommandGroupContinueOnError(id string, pri level.Priority, cmds [][]string, dir string, env map[string]string) *Command {
	return NewCommand().ID(id).Priority(pri).Extend(cmds).Directory(dir).Environment(env).ContinueOnError(true)
}

// BuildRemoteCommandGroupContinueOnError runs the group of sub-commands remotely
// given the configuration of arguments, continuing execution despite any
// errors.
func BuildRemoteCommandGroupContinueOnError(id string, pri level.Priority, host string, cmds [][]string, dir string) *Command {
	return NewCommand().ID(id).Priority(pri).Host(host).Extend(cmds).Directory(dir).ContinueOnError(true)
}

// BuildCommandGroup runs the group of sub-commands given the configuration of
// arguments.
func BuildCommandGroup(id string, pri level.Priority, cmds [][]string, dir string, env map[string]string) *Command {
	return NewCommand().ID(id).Priority(pri).Extend(cmds).Directory(dir).Environment(env)
}

// BuildRemoteCommandGroup runs the group of sub-commands remotely given the
// configuration of arguments.
func BuildRemoteCommandGroup(id string, pri level.Priority, host string, cmds [][]string, dir string) *Command {
	return NewCommand().ID(id).Priority(pri).Host(host).Extend(cmds).Directory(dir)
}

// BuildParallelCommandGroup runs the group of sub-commands in
// parallel given the configuration of arguments.
func BuildParallelCommandGroup(id string, pri level.Priority, cmds [][]string, dir string, env map[string]string) *Command {
	return NewCommand().ID(id).Priority(pri).Extend(cmds).Directory(dir).Environment(env)
}

// BuildParallelRemoteCommandGroup runs the group of sub-commands
// remotely in parallel given the configuration of arguments.
func BuildParallelRemoteCommandGroup(id string, pri level.Priority, host string, cmds [][]string, dir string) *Command {
	return NewCommand().ID(id).Priority(pri).Host(host).Extend(cmds).Directory(dir)
}
