package command

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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

	logger.Execution().Debug("Preparing script...")

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
		CmdString:  c.Script,
		Stdout:     logWriterInfo,
		Stderr:     logWriterErr,
		ScriptMode: true,
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

	if c.WorkingDir != "" {
		localCmd.WorkingDirectory = filepath.Join(conf.WorkDir, c.WorkingDir)
	} else {
		localCmd.WorkingDirectory = conf.WorkDir
	}

	if c.Shell != "" {
		localCmd.Shell = c.Shell
	}

	err := localCmd.PrepToRun(conf.Expansions)
	if err != nil {
		return errors.Wrap(err, "Failed to apply expansions")
	}

	if c.Silent {
		logger.Execution().Infof("Executing script with %s (source hidden)...",
			localCmd.Shell)
	} else {
		logger.Execution().Infof("Executing script with %s: %v",
			localCmd.Shell, localCmd.CmdString)
	}

	doneStatus := make(chan error)
	go func() {
		var err error
		env := os.Environ()
		env = append(env, fmt.Sprintf("%s=%s", subprocess.MarkerTaskID, conf.Task.Id),
			fmt.Sprintf("%s=%d", subprocess.MarkerAgentPID, os.Getpid()))
		localCmd.Environment = env
		err = localCmd.Start()

		if err != nil {
			logger.System().Debugf("error spawning shell process: %v", err)
		} else {
			logger.System().Debugf("spawned shell process with pid %d", localCmd.Cmd.Process.Pid)

			// Call the platform's process-tracking function. On some OSes this will be a noop,
			// on others this may need to do some additional work to track the process so that
			// it can be cleaned up later.
			subprocess.TrackProcess(conf.Task.Id, localCmd.Cmd.Process.Pid, logger.System())

			if c.Background {
				logger.Execution().Debug("running command in the background")
				close(doneStatus)
			} else {
				select {
				case doneStatus <- localCmd.Cmd.Wait():
					logger.System().Debugf("shell process %d completed", localCmd.Cmd.Process.Pid)
				case <-ctx.Done():
					doneStatus <- localCmd.Stop()
					logger.System().Infof("shell process %d terminated", localCmd.Cmd.Process.Pid)
				}
			}
		}
	}()

	select {
	case err = <-doneStatus:
		if err != nil {
			if c.ContinueOnError {
				logger.Execution().Infof("(ignoring) Script finished with error: %v", err)
				return nil
			}

			err = errors.Wrap(err, "script finished with error")
			logger.Execution().Info(err)
			return err
		}

		logger.Execution().Info("Script execution complete.")
	case <-ctx.Done():
		logger.Execution().Info("Got kill signal")

		// need to check command has started
		if localCmd.Cmd.Process != nil {
			logger.Execution().Infof("Stopping process: %d", localCmd.Cmd.Process.Pid)

			// try and stop the process
			if err := localCmd.Stop(); err != nil {
				logger.Execution().Error(errors.Wrap(err, "error while stopping process"))
			}
		}

		return errors.New("shell command interrupted")
	}

	return nil
}
