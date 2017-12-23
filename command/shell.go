package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
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

	if c.IgnoreStandardOutput && c.RedirectStandardErrorToOutput {
		return errors.New("cannot ignore standard out, and redirect standard error to it")
	}

	return nil
}

// Execute starts the shell with its given parameters.
func (c *shellExec) Execute(ctx context.Context,
	_ client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

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

	var logWriterInfo io.WriteCloser
	var logWriterErr io.WriteCloser

	if c.SystemLog {
		logWriterInfo = logger.SystemWriter(level.Info)
		logWriterErr = logger.SystemWriter(level.Error)
	} else {
		logWriterInfo = logger.TaskWriter(level.Info)
		logWriterErr = logger.TaskWriter(level.Error)
	}
	defer logWriterInfo.Close()
	defer logWriterErr.Close()

	localCmd := &subprocess.LocalCommand{
		CmdString:        c.Script,
		Stdout:           logWriterInfo,
		Stderr:           logWriterErr,
		WorkingDirectory: c.WorkingDir,
		ScriptMode:       true,
	}

	if c.IgnoreStandardError {
		localCmd.Stderr = nil
	}
	if c.IgnoreStandardOutput {
		localCmd.Stdout = nil
	}
	if c.RedirectStandardErrorToOutput {
		localCmd.Stderr = logWriterInfo
	}

	if c.Shell != "" {
		localCmd.Shell = c.Shell
	}

	if c.Silent {
		logger.Execution().Infof("Executing script with %s (source hidden)...",
			localCmd.Shell)
	} else {
		logger.Execution().Infof("Executing script with %s: %v",
			localCmd.Shell, localCmd.CmdString)
	}

	localCmd.Environment = append(os.Environ(), fmt.Sprintf("%s=%s", subprocess.MarkerTaskID, conf.Task.Id),
		fmt.Sprintf("%s=%d", subprocess.MarkerAgentPID, os.Getpid()))

	if err = localCmd.Start(ctx); err != nil {
		logger.System().Debugf("error spawning shell process: %v", err)
		return err
	}

	logger.System().Debugf("spawned shell process with pid %d", localCmd.GetPid())

	// Call the platform's process-tracking function. On some OSes this will be a noop,
	// on others this may need to do some additional work to track the process so that
	// it can be cleaned up later.
	subprocess.TrackProcess(conf.Task.Id, localCmd.GetPid(), logger.System())

	if c.Background {
		logger.Execution().Debug("running command in the background")
		return
	}

	err = errors.Wrap(localCmd.Wait(), "command encountered problem")
	if ctx.Err() != nil {
		logger.System().Debug("dumping running processes before canceling work")
		logger.System().Debug(message.CollectAllProcesses())
		logger.Execution().Notice(err)
		return errors.New("shell command interrupted")
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
