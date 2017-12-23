package command

import (
	"context"
	"io"
	"os"
	"strconv"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
)

type simpleExec struct {
	CommandName string            `mapstructure:"command_name"`
	Args        []string          `mapstructure:"args"`
	Env         map[string]string `mapstructure:"env"`

	Command string `mapstructure:"command"`

	// Background, if set to true, prevents shell code/output from
	// waiting for the script to complete and immediately returns
	// to the caller
	Background bool `mapstructure:"background"`

	// Silent, if set to true, prevents shell code/output from being
	// logged to the agent's task logs. This can be used to avoid
	// exposing sensitive expansion parameters and keys.
	Silent bool `mapstructure:"silent"`

	// SystemLog if set will write the shell command's output to the system logs, instead of the
	// task logs. This can be used to collect diagnostic data in the background of a running task.
	SystemLog bool `mapstructure:"system_log"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// IgnoreStandardOutput and IgnoreStandardError allow users to
	// elect to ignore either standard out and/or standard output.
	IgnoreStandardOutput bool `mapstructure:"ignore_standard_out"`
	IgnoreStandardError  bool `mapstructure:"ignore_standard_error"`

	// RedirectStandardErrorToOutput allows you to capture
	// standard error in the same stream as standard output. This
	// improves the synchronization of these streams.
	RedirectStandardErrorToOutput bool `mapstructure:"redirect_standard_error_to_output"`

	// ContinueOnError determines whether or not a failed return code
	// should cause the task to be marked as failed. Setting this to true
	// allows following commands to execute even if this shell command fails.
	ContinueOnError bool `mapstructure:"continue_on_err"`

	base
}

func simpleExecFactory() Command   { return &simpleExec{} }
func (c *simpleExec) Name() string { return "simple.exec" }

func (c *simpleExec) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrapf(err, "error decoding %v params", c.Name())
	}

	if c.Command != "" {
		if c.CommandName != "" || len(c.Args) > 0 {
			return errors.New("must specify command as either arguments or a command string but not both")
		}

		args, err := shlex.Split(c.Command)
		if err != nil || len(args) == 0 {
			return errors.Wrap(err, "problem parsing shell command")
		}

		c.CommandName = args[0]
		if len(args) > 1 {
			c.Args = args[1:]
		}
	}

	if c.Silent {
		c.IgnoreStandardError = true
		c.IgnoreStandardOutput = true
	}

	if c.IgnoreStandardOutput && c.RedirectStandardErrorToOutput {
		return errors.New("cannot ignore standard out, and redirect standard error to it")
	}

	if c.Env == nil {
		c.Env = make(map[string]string)
	}

	return nil
}

func (c *simpleExec) doExpansions(exp *util.Expansions) error {
	var err error
	catcher := grip.NewBasicCatcher()

	c.WorkingDir, err = exp.ExpandString(c.WorkingDir)
	catcher.Add(err)

	c.CommandName, err = exp.ExpandString(c.CommandName)
	catcher.Add(err)

	for idx := range c.Args {
		c.Args[idx], err = exp.ExpandString(c.Args[idx])
		catcher.Add(err)
	}

	for k, v := range c.Env {
		c.Env[k], err = exp.ExpandString(v)
		catcher.Add(err)
	}

	return errors.Wrap(catcher.Resolve(), "problem expanding strings")
}

func (c *simpleExec) getProc(taskID string, logger client.LoggerProducer) (subprocess.Command, func(), error) {
	c.Env[subprocess.MarkerTaskID] = taskID
	c.Env[subprocess.MarkerAgentPID] = strconv.Itoa(os.Getpid())

	proc, err := subprocess.NewLocalExec(c.CommandName, c.Args, c.Env, c.WorkingDir)
	if err != nil {
		return nil, nil, errors.Wrap(err, "problem constructing command wrapper")
	}

	var output io.WriteCloser
	var error io.WriteCloser
	var closer func()

	if c.SystemLog {
		output = logger.SystemWriter(level.Info)
		error = logger.SystemWriter(level.Error)
	} else {
		output = logger.TaskWriter(level.Info)
		error = logger.TaskWriter(level.Error)
	}

	opts := subprocess.OutputOptions{
		Output:            output,
		Error:             error,
		SuppressOutput:    c.IgnoreStandardOutput,
		SuppressError:     c.IgnoreStandardError,
		SendOutputToError: c.RedirectStandardErrorToOutput,
	}

	proc.SetOutput(opts)
	closer = func() {
		output.Close()
		error.Close()
	}

	return proc, closer, nil
}

func (c *simpleExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	var err error

	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Error("problem expanding command values")
	}

	c.WorkingDir, err = conf.GetWorkingDirectory(c.WorkingDir)
	if err != nil {
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}

	proc, closer, err := c.getProc(conf.Task.Id, logger)
	if err != nil {
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}
	defer closer()
	grip.Info(proc)

	err = errors.WithStack(c.runCommand(ctx, conf.Task.Id, proc, logger))

	if ctx.Err() != nil {
		logger.System().Debug("dumping running processes")
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)

		return errors.New("simple.exec aborted")
	}

	return err
}

func (c *simpleExec) runCommand(ctx context.Context, taskID string, proc subprocess.Command, logger client.LoggerProducer) error {
	if c.Silent {
		logger.Execution().Info("executing command in silent mode")
	}

	if err := proc.Start(ctx); err != nil {
		return errors.Wrap(err, "problem starting background process")
	}
	pid := proc.GetPid()
	subprocess.TrackProcess(taskID, pid, logger.System())

	if c.Background {
		logger.Execution().Infof("started background process with pid %d", pid)
		return nil
	}
	logger.Execution().Debugf("started foreground process with pid %d, waiting for completion", pid)

	err := errors.Wrapf(proc.Wait(), "command with pid %s encountered error", pid)

	if c.ContinueOnError {
		logger.Execution.Notice(err)
		return nil
	}

	return err
}
