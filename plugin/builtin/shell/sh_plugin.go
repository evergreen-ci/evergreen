package shell

import (
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/mapstructure"
	"net/http"
	"path/filepath"
)

func init() {
	plugin.Publish(&ShellPlugin{})
}

const (
	ShellPluginName = "shell"
	ShellExecCmd    = "exec"
)

// ShellPlugin runs arbitrary shell code on the agent's machine.
type ShellPlugin struct {
}

// Name returns the name of the plugin. Required to fulfill
// the Plugin interface.
func (self *ShellPlugin) Name() string {
	return ShellPluginName
}

// GetRoutes is required by the Plugin interface. This plugin has no API routes.
func (self *ShellPlugin) GetAPIHandler() http.Handler {
	//shell plugin doesn't need any server-side handlers, so this is a no-op
	return nil
}

func (self *ShellPlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *ShellPlugin) Configure(map[string]interface{}) error {
	return nil
}

// GetPanelConfig is required by the Plugin interface. This plugin has
// no UI presence.
func (self *ShellPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return nil, nil
}

// NewCommand returns the requested command, or returns an error
// if a non-existing command is requested.
func (self *ShellPlugin) NewCommand(cmdName string) (plugin.Command, error) {
	if cmdName != ShellExecCmd {
		return nil, fmt.Errorf("No such command: %v", cmdName)
	}
	return &ShellExecCommand{}, nil
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

// ParseParams reads in the command's parameters.
func (self *ShellExecCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, self)
	if err != nil {
		return fmt.Errorf("error decoding %v params: %v", self.Name(), err)
	}
	return nil
}

// Execute starts the shell with its given parameters.
func (self *ShellExecCommand) Execute(pluginLogger plugin.PluginLogger,
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

	command := &command.LocalCommand{
		CmdString:  self.Script,
		Stdout:     outBufferWriter,
		Stderr:     errorBufferWriter,
		ScriptMode: true,
	}

	if self.WorkingDir != "" {
		command.WorkingDirectory = filepath.Join(conf.WorkDir, self.WorkingDir)
	} else {
		command.WorkingDirectory = conf.WorkDir
	}

	err := command.PrepToRun(conf.Expansions)
	if err != nil {
		return fmt.Errorf("Failed to apply expansions: %v", err)
	}
	if self.Silent {
		pluginLogger.LogExecution(slogger.INFO, "Executing script (source hidden)...")
	} else {
		pluginLogger.LogExecution(slogger.INFO, "Executing script: %v", command.CmdString)
	}

	doneStatus := make(chan error)
	go func() {
		var err error
		if self.Background {
			err = command.Start()
		} else {
			err = command.Run()
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
	case _ = <-stop:
		//try and kill the process
		pluginLogger.LogExecution(slogger.INFO, "Got kill signal, stopping process: %v", command.Cmd.Process.Pid)
		if err := command.Stop(); err != nil {
			pluginLogger.LogExecution(slogger.ERROR, "Error occurred stopping process: %v", err)
		}
		return fmt.Errorf("Shell command interrupted.")
	}

	return nil
}
