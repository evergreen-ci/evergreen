package command

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
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
	// AddToPath allows additional paths to be prepended to the PATH environment
	// variable.
	AddToPath []string `mapstructure:"add_to_path"`

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
func (*shellExec) Name() string { return evergreen.ShellExecCommandName }

// ParseParams reads in the command's parameters.
func (c *shellExec) ParseParams(params map[string]any) error {
	if params == nil {
		return errors.New("params cannot be nil")
	}

	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Silent {
		c.IgnoreStandardError = true
		c.IgnoreStandardOutput = true
	}
	if c.Script == "" {
		return errors.New("must specify a script")
	}
	if c.Shell == "" {
		c.Shell = "sh"
	}

	if c.IgnoreStandardOutput && c.RedirectStandardErrorToOutput {
		return errors.New("cannot ignore standard output and also redirect standard error to it")
	}

	if c.Env == nil {
		c.Env = map[string]string{}
	}

	return nil
}

// Execute starts the shell with its given parameters.
func (c *shellExec) Execute(ctx context.Context, _ client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	logger.Execution().Debug("Preparing script...")

	// We do this before expanding expansions so that expansions are not logged.
	if c.Silent {
		logger.Execution().Infof("Executing script with shell '%s' (source hidden)...",
			c.Shell)
	} else {
		logger.Execution().Infof("Executing script with shell '%s':\n%s",
			c.Shell, c.Script)
	}

	var err error
	if err = c.doExpansions(&conf.Expansions); err != nil {
		return errors.WithStack(err)
	}

	logger.Execution().WarningWhen(filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		fmt.Sprintf("The working directory is an absolute path [%s], which isn't supported except when prefixed by '%s'.",
			c.WorkingDir, conf.WorkDir))

	c.WorkingDir, err = getWorkingDirectoryLegacy(conf, c.WorkingDir)
	if err != nil {
		return errors.Wrap(err, "getting working directory")
	}

	taskTmpDir, err := getWorkingDirectoryLegacy(conf, "tmp")
	if err != nil {
		logger.Execution().Notice(errors.Wrap(err, "getting task temporary directory"))
	}

	c.Env = defaultAndApplyExpansionsToEnv(c.Env, modifyEnvOptions{
		taskID:                 conf.Task.Id,
		workingDir:             c.WorkingDir,
		tmpDir:                 taskTmpDir,
		expansions:             conf.Expansions,
		includeExpansionsInEnv: c.IncludeExpansionsInEnv,
		addExpansionsToEnv:     c.AddExpansionsToEnv,
		addToPath:              c.AddToPath,
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

			return runJasperProcess(lctx, c.JasperManager(), c.Background, opts, conf.Task.Id, logger)
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

	if conf.Distro != nil {
		if execUser := conf.Distro.ExecUser; execUser != "" {
			cmd.SudoAs(execUser)
		}
	}

	err = cmd.Run(ctx)
	if !c.Background && err != nil {
		if exitCode, _ := cmd.Wait(ctx); exitCode != 0 {
			err = errors.Errorf("exit code %d", exitCode)
		}
	}
	err = errors.Wrapf(err, "shell script encountered problem")
	if ctxErr := ctx.Err(); ctxErr != nil {
		logger.System().Debugf("Canceled command '%s', dumping running processes.", c.Name())
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)
		return errors.Wrapf(ctxErr, "canceled while running command '%s'", c.Name())
	}

	if c.ContinueOnError && err != nil {
		logger.Execution().Noticef("Script errored, but continue on error is set - continuing task execution. Error: %s.", err)
		return nil
	}

	return err
}

func (c *shellExec) doExpansions(exp *util.Expansions) error {
	catcher := grip.NewBasicCatcher()
	var err error

	c.WorkingDir, err = exp.ExpandString(c.WorkingDir)
	catcher.Wrap(err, "expanding working directory")

	c.Script, err = exp.ExpandString(c.Script)
	catcher.Wrap(err, "expanding script")

	c.Shell, err = exp.ExpandString(c.Shell)
	catcher.Wrap(err, "expanding shell")

	for k, v := range c.Env {
		c.Env[k], err = exp.ExpandString(v)
		catcher.Wrapf(err, "expanding environment variable '%s'", k)
	}

	for idx := range c.AddToPath {
		c.AddToPath[idx], err = exp.ExpandString(c.AddToPath[idx])
		catcher.Wrapf(err, "expanding element %d to add to path", idx)
	}

	return catcher.Resolve()
}
