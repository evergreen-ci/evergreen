package jasper

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/shlex"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// Command objects allow a quick and lightweight interface for firing off
// ad-hoc processes for smaller tasks. Command immediately supports features
// such as output and error functionality and remote execution. Command methods
// are not thread-safe.
type Command struct {
	opts     options.Command
	procs    []Process
	runFunc  func(options.Command) error
	makeProc ProcessConstructor
}

func (c *Command) sudoCmd() []string {
	sudoCmd := []string{}
	if c.opts.Sudo {
		sudoCmd = append(sudoCmd, "sudo")
		if c.opts.SudoUser != "" {
			sudoCmd = append(sudoCmd, "-u", c.opts.SudoUser)
		}
	}
	return sudoCmd
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
	return &Command{makeProc: newBasicProcess}
}

// ProcConstructor returns a blank Command that will use the process created
// by the given ProcessConstructor.
func (c *Command) ProcConstructor(processConstructor ProcessConstructor) *Command {
	c.makeProc = processConstructor
	return c
}

// SetRunFunc sets the function that overrides the default behavior when a
// command is run, allowing the caller to run the command with their own custom
// function given all the given inputs to the command.
func (c *Command) SetRunFunc(f func(options.Command) error) *Command { c.runFunc = f; return c }

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

// ApplyFromOpts uses the options.Create to configure the Command. All existing
// options will be overwritten. Use of this function is discouraged unless all
// desired options are populated in the given opts.
// If Args is set on the options.Create, it will be ignored; the command
// arguments can be added using Add, Append, AppendArgs, or Extend.
func (c *Command) ApplyFromOpts(opts *options.Create) *Command {
	c.opts.Process = *opts
	return c
}

// SetOutputOptions sets the output options for a command. This overwrites an
// existing output options.
func (c *Command) SetOutputOptions(opts options.Output) *Command {
	c.opts.Process.Output = opts
	return c
}

// String returns a stringified representation.
func (c *Command) String() string {
	var remote string
	if c.opts.Remote == nil {
		remote = "nil"
	} else {
		remote = c.opts.Remote.String()
	}

	return fmt.Sprintf("id='%s', remote='%s', cmd='%s'", c.opts.ID, remote, c.getCmd())
}

// Export returns all of the options.Create that will be used to spawn the
// processes that run all subcommands.
func (c *Command) Export() ([]*options.Create, error) {
	opts, err := c.getCreateOpts()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting process creation options")
	}
	return opts, nil
}

func (c *Command) initRemote() {
	if c.opts.Remote == nil {
		c.opts.Remote = &options.Remote{}
	}
}

func (c *Command) initRemoteProxy() {
	if c.opts.Remote == nil {
		c.opts.Remote = &options.Remote{}
	}
	if c.opts.Remote.Proxy == nil {
		c.opts.Remote.Proxy = &options.Proxy{}
	}
}

// Host sets the hostname for connecting to a remote host.
func (c *Command) Host(h string) *Command {
	c.initRemote()
	c.opts.Remote.Host = h
	return c
}

// User sets the username for connecting to a remote host.
func (c *Command) User(u string) *Command {
	c.initRemote()
	c.opts.Remote.User = u
	return c
}

// Port sets the port for connecting to a remote host.
func (c *Command) Port(p int) *Command {
	c.initRemote()
	c.opts.Remote.Port = p
	return c
}

// ExtendRemoteArgs allows you to add arguments, when needed, to the
// Password sets the password in order to authenticate to a remote host.
// underlying ssh command, for remote commands.
func (c *Command) ExtendRemoteArgs(args ...string) *Command {
	c.initRemote()
	c.opts.Remote.Args = append(c.opts.Remote.Args, args...)
	return c
}

// PrivKey sets the private key in order to authenticate to a remote host.
func (c *Command) PrivKey(key string) *Command {
	c.initRemote()
	c.opts.Remote.Key = key
	return c
}

// PrivKeyFile sets the path to the private key file in order to authenticate to
// a remote host.
func (c *Command) PrivKeyFile(path string) *Command {
	c.initRemote()
	c.opts.Remote.KeyFile = path
	return c
}

// PrivKeyPassphrase sets the passphrase for the private key file in order to
// authenticate to a remote host.
func (c *Command) PrivKeyPassphrase(pass string) *Command {
	c.initRemote()
	c.opts.Remote.KeyPassphrase = pass
	return c
}

// Password sets the password in order to authenticate to a remote host.
func (c *Command) Password(p string) *Command {
	c.initRemote()
	c.opts.Remote.Password = p
	return c
}

// ProxyHost sets the proxy hostname for connecting to a proxy host.
func (c *Command) ProxyHost(h string) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.Host = h
	return c
}

// ProxyUser sets the proxy username for connecting to a proxy host.
func (c *Command) ProxyUser(u string) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.User = u
	return c
}

// ProxyPort sets the proxy port for connecting to a proxy host.
func (c *Command) ProxyPort(p int) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.Port = p
	return c
}

// ProxyPrivKey sets the proxy private key in order to authenticate to a remote host.
func (c *Command) ProxyPrivKey(key string) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.Key = key
	return c
}

// ProxyPrivKeyFile sets the path to the proxy private key file in order to
// authenticate to a proxy host.
func (c *Command) ProxyPrivKeyFile(path string) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.KeyFile = path
	return c
}

// ProxyPrivKeyPassphrase sets the passphrase for the private key file in order to
// authenticate to a proxy host.
func (c *Command) ProxyPrivKeyPassphrase(pass string) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.KeyPassphrase = pass
	return c
}

// ProxyPassword sets the password in order to authenticate to a proxy host.
func (c *Command) ProxyPassword(p string) *Command {
	c.initRemoteProxy()
	c.opts.Remote.Proxy.Password = p
	return c
}

// SetRemoteOptions sets the configuration for remote operations. This overrides
// any existing remote configuration.
func (c *Command) SetRemoteOptions(opts *options.Remote) *Command { c.opts.Remote = opts; return c }

// Directory sets the working directory. If this is a remote command, it sets
// the working directory of the command being run remotely.
func (c *Command) Directory(d string) *Command { c.opts.Process.WorkingDirectory = d; return c }

// Priority sets the logging priority.
func (c *Command) Priority(l level.Priority) *Command { c.opts.Priority = l; return c }

// ID sets the ID of the Command, which is independent of the IDs of the
// subcommands that are executed.
func (c *Command) ID(id string) *Command { c.opts.ID = id; return c }

// SetTags overrides any existing tags for a process with the
// specified list. Tags are used to filter process with the manager.
func (c *Command) SetTags(tags []string) *Command { c.opts.Process.Tags = tags; return c }

// AppendTags adds the specified tags to the existing tag slice. Tags
// are used to filter process with the manager.
func (c *Command) AppendTags(t ...string) *Command {
	c.opts.Process.Tags = append(c.opts.Process.Tags, t...)
	return c
}

// ExtendTags adds all tags in the specified slice to the tags will be
// added to the process after creation. Tags are used to filter
// process with the manager.
func (c *Command) ExtendTags(t []string) *Command {
	c.opts.Process.Tags = append(c.opts.Process.Tags, t...)
	return c
}

// Background allows you to set the command to run in the background
// when you call Run(), the command will begin executing but will not
// complete when run returns.
func (c *Command) Background(runBackground bool) *Command {
	c.opts.RunBackground = runBackground
	return c
}

// ContinueOnError sets a flag for determining if the Command should continue
// executing its sub-commands even if one of them errors.
func (c *Command) ContinueOnError(cont bool) *Command { c.opts.ContinueOnError = cont; return c }

// IgnoreError sets a flag for determining if the Command should return a nil
// error despite errors in its sub-command executions.
func (c *Command) IgnoreError(ignore bool) *Command { c.opts.IgnoreError = ignore; return c }

// SuppressStandardError sets a flag for determining if the Command should
// discard all standard error content.
func (c *Command) SuppressStandardError(v bool) *Command {
	c.opts.Process.Output.SuppressError = v
	return c
}

// SetLoggers sets the logging output on this command to the specified
// slice. This removes any loggers previously configured.
func (c *Command) SetLoggers(l []options.Logger) *Command {
	c.opts.Process.Output.Loggers = l
	return c
}

// AppendLoggers adds one or more loggers to the existing configured
// loggers in the command.
func (c *Command) AppendLoggers(l ...options.Logger) *Command {
	c.opts.Process.Output.Loggers = append(c.opts.Process.Output.Loggers, l...)
	return c
}

// ExtendLoggers takes the existing slice of loggers and adds that to any
// existing configuration.
func (c *Command) ExtendLoggers(l []options.Logger) *Command {
	c.opts.Process.Output.Loggers = append(c.opts.Process.Output.Loggers, l...)
	return c
}

// SuppressStandardOutput sets a flag for determining if the Command should
// discard all standard output content.
func (c *Command) SuppressStandardOutput(v bool) *Command {
	c.opts.Process.Output.SuppressOutput = v
	return c
}

// RedirectOutputToError sets a flag for determining if the Command should send
// all standard output content to standard error.
func (c *Command) RedirectOutputToError(v bool) *Command {
	c.opts.Process.Output.SendOutputToError = v
	return c
}

// RedirectErrorToOutput sets a flag for determining if the Command should send
// all standard error content to standard output.
func (c *Command) RedirectErrorToOutput(v bool) *Command {
	c.opts.Process.Output.SendErrorToOutput = v
	return c
}

// Environment replaces the current environment map with the given environment
// map. If this is a remote command, it sets the environment of the command
// being run remotely.
func (c *Command) Environment(e map[string]string) *Command {
	c.opts.Process.Environment = e
	return c
}

// AddEnv adds a key value pair of environment variable to value into the
// Command's environment variable map. If this is a remote command, it sets the
// environment of the command being run remotely.
func (c *Command) AddEnv(k, v string) *Command {
	c.setupEnv()
	c.opts.Process.Environment[k] = v
	return c
}

// Prerequisite sets a function on the Command such that the Command will only
// execute if the function returns true.
func (c *Command) Prerequisite(chk func() bool) *Command { c.opts.Prerequisite = chk; return c }

// Add adds on a sub-command.
func (c *Command) Add(args []string) *Command {
	c.opts.Commands = append(c.opts.Commands, args)
	return c
}

// Sudo runs each command with superuser privileges with the default target
// user. This will cause the commands to fail if the commands are executed in
// Windows. If this is a remote command, the command being run remotely uses
// superuser privileges.
func (c *Command) Sudo(sudo bool) *Command { c.opts.Sudo = sudo; return c }

// SudoAs runs each command with sudo but allows each command to be run as a
// user other than the default target user (usually root). This will cause the
// commands to fail if the commands are executed in Windows. If this is a remote
// command, the command being run remotely uses superuser privileges.
func (c *Command) SudoAs(user string) *Command {
	c.opts.Sudo = true
	c.opts.SudoUser = user
	return c
}

// Extend adds on multiple sub-commands.
func (c *Command) Extend(cmds [][]string) *Command {
	c.opts.Commands = append(c.opts.Commands, cmds...)
	return c
}

// Append takes a series of strings and splits them into sub-commands and adds
// them to the Command.
func (c *Command) Append(cmds ...string) *Command {
	for _, cmd := range cmds {
		c.opts.Commands = append(c.opts.Commands, splitCmdToArgs(cmd))
	}
	return c
}

// ShellScript adds an operation to the command that runs a shell script, using
// the shell's "-c" option).
func (c *Command) ShellScript(shell, script string) *Command {
	c.opts.Commands = append(c.opts.Commands, []string{shell, "-c", script})
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

// SetHook allows you to add a function that's always called (locally)
// after the command completes.
func (c *Command) SetHook(h func(error) error) *Command { c.opts.Hook = h; return c }

func (c *Command) setupEnv() {
	if c.opts.Process.Environment == nil {
		c.opts.Process.Environment = map[string]string{}
	}
}

// Run starts and then waits on the Command's execution.
func (c *Command) Run(ctx context.Context) error {
	if c.opts.Prerequisite != nil && !c.opts.Prerequisite() {
		grip.Debug(message.Fields{
			"op":  "noop after prerequisite returned false",
			"id":  c.opts.ID,
			"cmd": c.String(),
		})
		return nil
	}

	if c.runFunc != nil {
		return c.runFunc(c.opts)
	}

	catcher := grip.NewBasicCatcher()

	opts, err := c.getCreateOpts()
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
		catcher.AddWhen(!c.opts.IgnoreError, err)

		if c.opts.Hook != nil {
			catcher.AddWhen(!c.opts.IgnoreError, c.opts.Hook(err))
		}

		if err != nil && !c.opts.ContinueOnError {
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
	parallelCmds := make([]Command, len(c.opts.Commands))

	for idx, cmd := range c.opts.Commands {
		splitCmd := *c
		optsCopy := c.opts.Process.Copy()
		splitCmd.opts.Process = *optsCopy
		splitCmd.opts.Commands = [][]string{cmd}
		splitCmd.procs = []Process{}
		parallelCmds[idx] = splitCmd
	}

	type cmdResult struct {
		procs []Process
		err   error
	}
	cmdResults := make(chan cmdResult, len(c.opts.Commands))
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
	for i := 0; i < len(c.opts.Commands); i++ {
		select {
		case cmdRes := <-cmdResults:
			if !c.opts.IgnoreError {
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
	return c.opts.Process.Close()
}

// SetInput sets the standard input.
func (c *Command) SetInput(r io.Reader) *Command {
	c.opts.Process.StandardInput = r
	return c
}

// SetInputBytes is the same as SetInput but sets b as the bytes to be read from
// standard input.
func (c *Command) SetInputBytes(b []byte) *Command {
	c.opts.Process.StandardInputBytes = b
	return c
}

// SetErrorSender sets a Sender to be used by this Command for its output to
// stderr.
func (c *Command) SetErrorSender(l level.Priority, s send.Sender) *Command {
	writer := send.MakeWriterSender(s, l)
	c.opts.Process.RegisterCloser(writer.Close)
	c.opts.Process.Output.Error = writer
	return c
}

// SetOutputSender sets a Sender to be used by this Command for its output to
// stdout.
func (c *Command) SetOutputSender(l level.Priority, s send.Sender) *Command {
	writer := send.MakeWriterSender(s, l)
	c.opts.Process.RegisterCloser(writer.Close)
	c.opts.Process.Output.Output = writer
	return c
}

// SetCombinedSender is the combination of SetErrorSender() and
// SetOutputSender().
func (c *Command) SetCombinedSender(l level.Priority, s send.Sender) *Command {
	writer := send.MakeWriterSender(s, l)
	c.opts.Process.RegisterCloser(writer.Close)
	c.opts.Process.Output.Error = writer
	c.opts.Process.Output.Output = writer
	return c
}

// SetErrorWriter sets a Writer to be used by this Command for its output to
// stderr.
func (c *Command) SetErrorWriter(writer io.WriteCloser) *Command {
	c.opts.Process.RegisterCloser(writer.Close)
	c.opts.Process.Output.Error = writer
	return c
}

// SetOutputWriter sets a Writer to be used by this Command for its output to
// stdout.
func (c *Command) SetOutputWriter(writer io.WriteCloser) *Command {
	c.opts.Process.RegisterCloser(writer.Close)
	c.opts.Process.Output.Output = writer
	return c
}

// SetCombinedWriter is the combination of SetErrorWriter() and
// SetOutputWriter().
func (c *Command) SetCombinedWriter(writer io.WriteCloser) *Command {
	c.opts.Process.RegisterCloser(writer.Close)
	c.opts.Process.Output.Error = writer
	c.opts.Process.Output.Output = writer
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
	opts, err := c.getCreateOpts()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out := make([]amboy.Job, len(opts))
	for idx := range opts {
		out[idx] = NewJobForeground(c.makeProc, opts[idx])
	}
	return out, nil
}

// Jobs returns a slice of jobs for every operation in the
// command. The output of the commands are captured in the body of the
// job.
func (c *Command) Jobs(ctx context.Context) ([]amboy.Job, error) {
	opts, err := c.getCreateOpts()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out := make([]amboy.Job, len(opts))
	for idx := range opts {
		out[idx] = NewJobOptions(c.makeProc, opts[idx])
	}
	return out, nil
}

func (c *Command) getCmd() string {
	env := strings.Join(c.opts.Process.ResolveEnvironment(), " ")
	out := []string{}
	for _, cmd := range c.opts.Commands {
		if c.opts.Sudo {
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

func (c *Command) getCreateOpt(args []string) (*options.Create, error) {
	opts := c.opts.Process.Copy()

	switch len(args) {
	case 0:
		return nil, errors.New("cannot have empty args")
	case 1:
		if c.opts.Remote == nil && strings.ContainsAny(args[0], " \"'") {
			spl, err := shlex.Split(args[0])
			if err != nil {
				return nil, errors.Wrap(err, "problem splitting argstring")
			}
			return c.getCreateOpt(spl)
		}
		opts.Args = args
	default:
		opts.Args = args
	}

	if c.opts.Sudo {
		opts.Args = append(c.sudoCmd(), opts.Args...)
	}

	return opts, nil
}

func (c *Command) getCreateOpts() ([]*options.Create, error) {
	out := []*options.Create{}
	catcher := grip.NewBasicCatcher()
	for _, args := range c.opts.Commands {
		cmd, err := c.getCreateOpt(args)
		if err != nil {
			catcher.Add(err)
			continue
		}

		if c.opts.Remote != nil {
			cmd.Remote = c.opts.Remote
		}

		out = append(out, cmd)
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return out, nil
}

func (c *Command) exec(ctx context.Context, opts *options.Create, idx int) error {
	msg := message.Fields{
		"id":         c.opts.ID,
		"cmd":        strings.Join(opts.Args, " "),
		"index":      idx,
		"len":        len(c.opts.Commands),
		"background": c.opts.RunBackground,
		"tags":       c.opts.Process.Tags,
	}

	writeOutput := getMsgOutput(opts.Output)
	proc, err := c.makeProc(ctx, opts)
	if err != nil {
		return errors.Wrap(err, "problem starting command")
	}
	c.procs = append(c.procs, proc)

	if !c.opts.RunBackground {
		waitCatcher := grip.NewBasicCatcher()
		for _, proc := range c.procs {
			_, err = proc.Wait(ctx)
			waitCatcher.Add(errors.Wrapf(err, "error waiting on process '%s'", proc.ID()))
		}
		err = waitCatcher.Resolve()
		msg["err"] = err
		grip.Log(c.opts.Priority, writeOutput(msg))
	}

	return errors.WithStack(err)
}

func getMsgOutput(opts options.Output) func(msg message.Fields) message.Fields {
	noOutput := func(msg message.Fields) message.Fields { return msg }

	if opts.Output != nil && opts.Error != nil {
		return noOutput
	}

	logger := NewInMemoryLogger(1000)
	sender, err := logger.Configure()
	if err != nil {
		return func(msg message.Fields) message.Fields {
			msg["log_err"] = errors.Wrap(err, "could not set up in-memory sender for capturing output")
			return msg
		}
	}

	writer := send.NewWriterSender(sender)
	if opts.Output == nil {
		opts.Output = writer
	}
	if opts.Error == nil {
		opts.Error = writer
	}

	return func(msg message.Fields) message.Fields {
		inMemorySender, ok := sender.(*send.InMemorySender)
		if !ok {
			msg["log_err"] = err
			return msg
		}
		logs, err := inMemorySender.GetString()
		if err != nil {
			msg["log_err"] = err
			return msg
		}
		msg["output"] = strings.Join(logs, "\n")
		return msg
	}
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
