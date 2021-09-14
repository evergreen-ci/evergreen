package command

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
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

	Env map[string]string `mapstructure:"env"`
	// AddExpansionsToEnv adds all defined expansions to the shell environment.
	AddExpansionsToEnv bool `mapstructure:"add_expansions_to_env"`
	// IncludeExpansionsInEnv allows users to specify which expansions should be
	// included in the environment, if they are defined. It is not an error to
	// specify expansions that are not defined in include_expansions_in_env.
	IncludeExpansionsInEnv []string `mapstructure:"include_expansions_in_env"`

	// Background, if set to true, prevents shell code/output from
	// waiting for the script to complete and immediately returns
	// to the caller
	Background bool `mapstructure:"background"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// SystemLog if set will write the shell command's output to the system logs, instead of the
	// task logs. This can be used to collect diagnostic data in the background of a running task.
	SystemLog bool `mapstructure:"system_log"`

	// ExecuteAsString forces the script to do something like `sh -c "<script arg>"`. By default this command
	// executes sh and passes the script arg to its stdin
	ExecuteAsString bool `mapstructure:"exec_as_string"`

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

	if c.Env == nil {
		c.Env = map[string]string{}
	}

	return nil
}

// Execute starts the shell with its given parameters.
func (c *shellExec) Execute(ctx context.Context, _ client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	logger.Execution().Debug("Preparing script...")

	var err error
	if err = c.doExpansions(conf.Expansions); err != nil {
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}

	logger.Execution().WarningWhen(filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		fmt.Sprintf("the working directory is an absolute path [%s], which isn't supported except when prefixed by '%s'",
			c.WorkingDir, conf.WorkDir))

	c.WorkingDir, err = conf.GetWorkingDirectory(c.WorkingDir)
	if err != nil {
		logger.Execution().Warning(err.Error())
		return errors.WithStack(err)
	}

	taskTmpDir, err := conf.GetWorkingDirectory("tmp")
	if err != nil {
		logger.Execution().Notice(err.Error())
	}

	var exp util.Expansions
	if conf.Expansions != nil {
		exp = *conf.Expansions
	}
	c.Env = defaultAndApplyExpansionsToEnv(c.Env, modifyEnvOptions{
		taskID:                 conf.Task.Id,
		workingDir:             c.WorkingDir,
		tmpDir:                 taskTmpDir,
		expansions:             exp,
		includeExpansionsInEnv: c.IncludeExpansionsInEnv,
		addExpansionsToEnv:     c.AddExpansionsToEnv,
	})

	logger.Execution().Debug(message.Fields{
		"working_directory": c.WorkingDir,
		"shell":             c.Shell,
	})

	cmd := c.JasperManager().CreateCommand(ctx).
		Background(c.Background).Directory(c.WorkingDir).Environment(c.Env).Append(c.Shell).
		SuppressStandardError(c.IgnoreStandardError).SuppressStandardOutput(c.IgnoreStandardOutput).RedirectErrorToOutput(c.RedirectStandardErrorToOutput).
		ProcConstructor(func(lctx context.Context, opts *options.Create) (jasper.Process, error) {
			if c.ExecuteAsString {
				opts.Args = append(opts.Args, "-c", c.Script)
			} else {
				opts.StandardInput = strings.NewReader(c.Script)
			}

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

			agentutil.TrackProcess(conf.Task.Id, pid, logger.System())

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

	c.Shell, err = exp.ExpandString(c.Shell)
	catcher.Add(err)

	for k, v := range c.Env {
		c.Env[k], err = exp.ExpandString(v)
		catcher.Add(err)
	}

	return catcher.Resolve()
}
