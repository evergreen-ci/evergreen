package command

import (
	"context"
	"os"
	"strconv"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

type subprocessExec struct {
	Binary  string            `mapstructure:"binary"`
	Args    []string          `mapstructure:"args"`
	Env     map[string]string `mapstructure:"env"`
	Command string            `mapstructure:"command"`

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

	// KeepEmptyArgs will allow empty arguments in commands if set to true
	// note that non-blank whitespace arguments are never stripped
	KeepEmptyArgs bool `mapstructure:"keep_empty_args"`

	base
}

func subprocessExecFactory() Command   { return &subprocessExec{} }
func (c *subprocessExec) Name() string { return "subprocess.exec" }

func (c *subprocessExec) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}

	if c.Command != "" {
		if c.Binary != "" || len(c.Args) > 0 {
			return errors.New("must specify command as either arguments or a command string but not both")
		}

		args, err := shlex.Split(c.Command)
		if err != nil || len(args) == 0 {
			return errors.Wrap(err, "problem parsing shell command")
		}

		c.Binary = args[0]
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

func (c *subprocessExec) doExpansions(exp *util.Expansions) error {
	var err error
	catcher := grip.NewBasicCatcher()

	c.WorkingDir, err = exp.ExpandString(c.WorkingDir)
	catcher.Add(err)

	c.Binary, err = exp.ExpandString(c.Binary)
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

func (c *subprocessExec) getProc(taskID string, logger client.LoggerProducer) *jasper.Command {
	c.Env[util.MarkerTaskID] = taskID
	c.Env[util.MarkerAgentPID] = strconv.Itoa(os.Getpid())

	cmd := c.JasperManager().CreateCommand().Add(append([]string{c.Binary}, c.Args...)).
		Background(c.Background).Environment(c.Env).Directory(c.WorkingDir).
		SuppressStandardError(c.IgnoreStandardError).SuppressStandardOutput(c.IgnoreStandardOutput).RedirectErrorToOutput(c.RedirectStandardErrorToOutput).
		ProcConstructor(func(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
			proc, err := c.JasperManager().CreateProcess(ctx, opts)
			if err != nil {
				return proc, errors.WithStack(err)
			}

			pid := proc.Info().PID
			logger.Exeuction().Infof("started process '%s' with pid '%d'", c.Binary, pid)
			util.TrackProcess(taskID, pid, logger.System())
			return proc, nil
		})

	if !opts.SuppressOutput {
		if c.SystemLog {
			cmd.SetOutputSender(level.Info, logger.System().GetSender())
		} else {
			cmd.SetOutputSender(level.Info, logger.Task().GetSender())
		}
	}

	if !opts.SuppressError {
		if c.SystemLog {
			cmd.SetErrorSender(level.Error, logger.System().GetSender())
		} else {
			cmd.SetErrorSender(level.Error, logger.Task().GetSender())
		}
	}

	return cmd
}

func addTempDirs(env map[string]string, dir string) {
	for _, key := range []string{"TMP", "TMPDIR", "TEMP"} {
		if _, ok := env[key]; ok {
			continue
		}
		env[key] = dir
	}
}

func (c *subprocessExec) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	var err error

	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Error("problem expanding command values")
		return errors.WithStack(err)
	}

	c.WorkingDir, err = conf.GetWorkingDirectory(c.WorkingDir)
	if err != nil {
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}

	taskTmpDir, err := conf.GetWorkingDirectory("tmp")
	if err != nil {
		logger.Execution().Notice(err.Error())
	}

	addTempDirs(c.Env, taskTmpDir)

	if !c.KeepEmptyArgs {
		for i, arg := range c.Args {
			if arg == "" {
				c.Args = append(c.Args[:i], c.Args[i+1:]...)
			}
		}
	}

	err = errors.WithStack(c.runCommand(ctx, conf.Task.Id, c.getProc(conf.Task.Id, logger), logger))

	if ctx.Err() != nil {
		logger.System().Debug("dumping running processes")
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)

		return errors.Errorf("%s aborted", c.Name())
	}

	return err
}

func (c *subprocessExec) runCommand(ctx context.Context, taskID string, cmd *jasper.Command, logger client.LoggerProducer) error {
	if c.Silent {
		logger.Execution().Info("executing command in silent mode")
	}

	err := cmd.Run(ctx)

	if c.Background {
		logger.Execution().Info("started background process '%s'", c.Binary)
		return nil
	}

	if c.ContinueOnError {
		logger.Execution().Notice(message.WrapError(err, message.Fields{
			"task":       taskID,
			"binary":     c.Binary,
			"background": c.Background,
			"silent":     c.Silent,
			"continue":   c.ContinueOnError,
			"command":    c.Command(),
		}))
		return nil
	}

	return err
}
