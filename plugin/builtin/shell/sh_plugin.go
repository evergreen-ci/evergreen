package shell

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

func init() {
	plugin.Publish(&ShellPlugin{})
}

const (
	ShellPluginName = "shell"
	ShellExecCmd    = "exec"
	CleanupCmd      = "cleanup"
	TrackCmd        = "track"
)

// ShellPlugin runs arbitrary shell code on the agent's machine.
type ShellPlugin struct{}

// Name returns the name of the plugin. Required to fulfill
// the Plugin interface.
func (self *ShellPlugin) Name() string {
	return ShellPluginName
}

// NewCommand returns the requested command, or returns an error
// if a non-existing command is requested.
func (self *ShellPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName == TrackCmd {
		return &TrackCommand{}, nil
	} else if cmdName == CleanupCmd {
		return &CleanupCommand{}, nil
	} else if cmdName == ShellExecCmd {
		return &ShellExecCommand{}, nil
	}
	return nil, fmt.Errorf("no such command: %v", cmdName)
}

type TrackCommand struct{}

func (cc *TrackCommand) Name() string {
	return TrackCmd
}

func (cc *TrackCommand) Plugin() string {
	return ShellPluginName
}

func (cc *TrackCommand) ParseParams(params map[string]interface{}) error {
	return nil
}

// Execute starts the shell with its given parameters.
func (cc *TrackCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	pluginLogger.LogExecution(slogger.WARN,
		"WARNING: shell.track is deprecated. Process tracking is now enabled by default.")
	return nil
}

type CleanupCommand struct{}

func (cc *CleanupCommand) Name() string {
	return CleanupCmd
}

func (cc *CleanupCommand) Plugin() string {
	return ShellPluginName
}

// ParseParams reads in the command's parameters.
func (cc *CleanupCommand) ParseParams(params map[string]interface{}) error {
	return nil
}

// Execute starts the shell with its given parameters.
func (cc *CleanupCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	pluginLogger.LogExecution(slogger.WARN,
		"WARNING: shell.cleanup is deprecated. Process cleanup is now enabled by default.")
	return nil
}

// ShellExecCommand is responsible for running the shell code.
type ShellExecCommand struct {
	// Script is the shell code to be run on the agent machine.
	Script string `mapstructure:"script" plugin:"expand"`

	// Silent, if set to true, prevents shell code/output from being
	// logged to the agent's task logs. This can be used to avoid
	// exposing sensitive expansion parameters and keys.
	Silent bool `mapstructure:"silent"`

	// Background, if set to true, prevents shell code/output from
	// waiting for the script to complete and immediately returns
	// to the caller
	Background bool `mapstructure:"background"`

	// WorkingDir is the working directory to start the shell in.
	WorkingDir string `mapstructure:"working_dir"`

	// ContinueOnError determines whether or not a failed return code
	// should cause the task to be marked as failed. Setting this to true
	// allows following commands to execute even if this shell command fails.
	ContinueOnError bool `mapstructure:"continue_on_err"`
}

func (self *ShellExecCommand) Name() string {
	return ShellExecCmd
}

func (self *ShellExecCommand) Plugin() string {
	return ShellPluginName
}

// ParseParams reads in the command's parameters.
func (self *ShellExecCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, self)
	if err != nil {
		return fmt.Errorf("error decoding %v params: %v", self.Name(), err)
	}
	return nil
}

// Execute starts the shell with its given parameters.
func (self *ShellExecCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	conf *model.TaskConfig,
	stop chan bool) error {
	pluginLogger.LogExecution(slogger.DEBUG, "Preparing script...")

	logWriterInfo := pluginLogger.GetTaskLogWriter(slogger.INFO)
	logWriterErr := pluginLogger.GetTaskLogWriter(slogger.ERROR)

	outBufferWriter := util.NewLineBufferingWriter(logWriterInfo)
	errorBufferWriter := util.NewLineBufferingWriter(logWriterErr)
	defer outBufferWriter.Flush()
	defer errorBufferWriter.Flush()

	localCmd := &command.LocalCommand{
		CmdString:  self.Script,
		Stdout:     outBufferWriter,
		Stderr:     errorBufferWriter,
		ScriptMode: true,
	}

	if self.WorkingDir != "" {
		localCmd.WorkingDirectory = filepath.Join(conf.WorkDir, self.WorkingDir)
	} else {
		localCmd.WorkingDirectory = conf.WorkDir
	}

	err := localCmd.PrepToRun(conf.Expansions)
	if err != nil {
		return fmt.Errorf("Failed to apply expansions: %v", err)
	}
	if self.Silent {
		pluginLogger.LogExecution(slogger.INFO, "Executing script (source hidden)...")
	} else {
		pluginLogger.LogExecution(slogger.INFO, "Executing script: %v", localCmd.CmdString)
	}

	doneStatus := make(chan error)
	go func() {
		var err error
		env := os.Environ()
		env = append(env, fmt.Sprintf("EVR_TASK_ID=%v", conf.Task.Id))
		env = append(env, fmt.Sprintf("EVR_AGENT_PID=%v", os.Getpid()))
		localCmd.Environment = env
		err = localCmd.Start()
		if err == nil {
			pluginLogger.LogSystem(slogger.DEBUG, "spawned shell process with pid %v", localCmd.Cmd.Process.Pid)

			// Call the platform's process-tracking function. On some OSes this will be a noop,
			// on others this may need to do some additional work to track the process so that
			// it can be cleaned up later.
			trackProcess(conf.Task.Id, localCmd.Cmd.Process.Pid, pluginLogger)

			if !self.Background {
				err = localCmd.Cmd.Wait()
			}

		} else {
			pluginLogger.LogSystem(slogger.DEBUG, "error spawning shell process: %v", err)
		}
		doneStatus <- err
	}()

	defer pluginLogger.Flush()
	select {
	case err = <-doneStatus:
		if err != nil {
			if self.ContinueOnError {
				pluginLogger.LogExecution(slogger.INFO, "(ignoring) Script finished with error: %v", err)
				return nil
			} else {
				pluginLogger.LogExecution(slogger.INFO, "Script finished with error: %v", err)
				return err
			}
		} else {
			pluginLogger.LogExecution(slogger.INFO, "Script execution complete.")
		}
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Got kill signal")

		// need to check command has started
		if localCmd.Cmd != nil {
			pluginLogger.LogExecution(slogger.INFO, "Stopping process: %v", localCmd.Cmd.Process.Pid)

			// try and stop the process
			if err := localCmd.Stop(); err != nil {
				pluginLogger.LogExecution(slogger.ERROR, "Error occurred stopping process: %v", err)
			}
		}

		return fmt.Errorf("Shell command interrupted.")
	}

	return nil
}

// listProc() returns a list of active pids on the system, by listing the contents of /proc
// and looking for entries that appear to be valid pids. Only usable on systems with a /proc
// filesystem (Solaris and UNIX/Linux)
func listProc() ([]int, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make([]int, 0, 50)
	for {
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, fi := range fis {
			// Pid must be a directory with a numeric name
			if !fi.IsDir() {
				continue
			}

			// Using Atoi here will also filter out . and ..
			pid, err := strconv.Atoi(fi.Name())
			if err != nil {
				continue
			}
			results = append(results, int(pid))
		}
	}
	return results, nil
}

// envHasMarkers returns a bool indicating if both marker vars are found in an environment var list
func envHasMarkers(env []string, pidMarker, taskMarker string) bool {
	hasPidMarker := false
	hasTaskMarker := false
	for _, envVar := range env {
		if envVar == pidMarker {
			hasPidMarker = true
		}
		if envVar == taskMarker {
			hasTaskMarker = true
		}
	}
	return hasPidMarker && hasTaskMarker
}

// KillSpawnedProcs cleans up any tasks that were spawned by the given task.
func KillSpawnedProcs(taskId string, pluginLogger plugin.Logger) error {
	// Clean up all shell processes spawned during the execution of this task by this agent,
	// by calling the platform-specific "cleanup" function
	return cleanup(taskId, pluginLogger)
}
