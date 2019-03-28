package command

import (
	"context"
	"os"
	"strconv"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// shellExec is responsible for running the shell code.
type shellExec struct {
	// Script is the shell code to be run on the agent machine.
	Script string `mapstructure:"script" plugin:"expand"`

	// Silent, if set to true, prevents shell code/output from being
	// logged to the agent's task logs. This can be used to avoid
	// exposing sensitive expansion parameters and keys.
	Silent bool `mapstructure:"silent"`

	// Shell describes the shell to execute the script contents
	// with. Defaults to "sh", but users can customize to
	// explicitly specify another shell.
	Shell string `mapstructure:"shell"`

	// Background, if set to true, prevents shell code/output from
	// waiting for the script to complete and immediately returns
	// to the caller
	Background bool `mapstructure:"background"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// SystemLog if set will write the shell command's output to the system logs, instead of the
	// task logs. This can be used to collect diagnostic data in the background of a running task.
	SystemLog bool `mapstructure:"system_log"`

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

func shellExecFactory() Command { return &shellExec{} }
func (*shellExec) Name() string { return "shell.exec" }

// ParseParams reads in the command's parameters.
func (c *shellExec) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrapf(err, "error decoding %v params", c.Name())
	}

	if c.Silent {
		c.IgnoreStandardError = true
		c.IgnoreStandardOutput = true
	}

	if c.Shell == "" {
		c.Shell = "sh"
	}

	if c.IgnoreStandardOutput && c.RedirectStandardErrorToOutput {
		return errors.New("cannot ignore standard out, and redirect standard error to it")
	}

	return nil
}

// Execute starts the shell with its given parameters.
func (c *shellExec) Execute(ctx context.Context, _ client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	logger.Execution().Debug("Preparing script...")

	var err error
	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Warning(err.Error())
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

	env := map[string]string{
		util.MarkerTaskID:   conf.Task.Id,
		util.MarkerAgentPID: strconv.Itoa(os.Getpid()),
	}
	addTempDirs(env, taskTmpDir)

	cmd := c.JasperManager().CreateCommand(ctx).Add([]string{c.Shell, "-c", c.Script}).
		Background(c.Background).Directory(c.WorkingDir).Environment(env).
		SuppressStandardError(c.IgnoreStandardError).SuppressStandardOutput(c.IgnoreStandardOutput).RedirectErrorToOutput(c.RedirectStandardErrorToOutput).
		ProcConstructor(func(lctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
			var cancel context.CancelFunc
			var ictx context.Context
			if c.Background {
				ictx, cancel = context.WithCancel(context.Background())
			} else {
				ictx = lctx
			}

			var proc jasper.Process
			proc, err = c.JasperManager().CreateProcess(ictx, opts)
			if err != nil {
				if cancel != nil {
					cancel()
				}

				return proc, errors.WithStack(err)
			}

			if cancel != nil {
				grip.Warning(message.WrapError(proc.RegisterTrigger(lctx, func(info jasper.ProcessInfo) {
					cancel()
				}), "problem registering cancellation for process"))
			}

			pid := proc.Info(ctx).PID

			util.TrackProcess(conf.Task.Id, pid, logger.System())

			if c.Background {
				logger.Execution().Debugf("running command in the background [pid=%d]", pid)
			} else {
				logger.Execution().Infof("started process with pid '%d'", pid)
			}

			return proc, nil
		})

	if !c.IgnoreStandardOutput {
		if c.SystemLog {
			cmd.SetOutputSender(level.Info, logger.System().GetSender())
		} else {
			cmd.SetOutputSender(level.Info, logger.Task().GetSender())
		}
	}

	if !c.IgnoreStandardError {
		if c.SystemLog {
			cmd.SetErrorSender(level.Error, logger.System().GetSender())
		} else {
			cmd.SetErrorSender(level.Error, logger.Task().GetSender())
		}
	}

	if c.Silent {
		logger.Execution().Infof("Executing script with %s (source hidden)...",
			c.Shell)
	} else {
		logger.Execution().Infof("Executing script with %s: %v",
			c.Shell, c.Script)
	}

	err = cmd.Run(ctx)

	err = errors.Wrapf(err, "command encountered problem")
	if ctx.Err() != nil {
		logger.System().Debug("dumping running processes before canceling work")
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)
		return errors.New("shell command interrupted")
	}

	if c.ContinueOnError {
		logger.Execution().Notice(err)
		return nil
	}

	return err
}

func (c *shellExec) doExpansions(exp *util.Expansions) error {
	catcher := grip.NewBasicCatcher()
	var err error

	c.WorkingDir, err = exp.ExpandString(c.WorkingDir)
	catcher.Add(err)

	c.Script, err = exp.ExpandString(c.Script)
	catcher.Add(err)

	return catcher.Resolve()
}
