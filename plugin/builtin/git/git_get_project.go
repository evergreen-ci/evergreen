package git

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"os"
	"path/filepath"
	"strings"
)

// GitGetProjectCommand is a command that fetches source code from git for the project
// associated with the current task
type GitGetProjectCommand struct {
	// The root directory (locally) that the code should be checked out into.
	// Must be a valid non-blank directory name.
	Directory string
}

func (self *GitGetProjectCommand) Name() string {
	return GetProjectCmdName
}

func (self *GitGetProjectCommand) Plugin() string {
	return GitPluginName
}

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (self *GitGetProjectCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, self)
	if err != nil {
		return err
	}

	if self.Directory == "" {
		return fmt.Errorf("error parsing '%v' params: value for directory "+
			"must not be blank", self.Name())
	}
	return nil
}

// Execute gets the source code required by the project
func (self *GitGetProjectCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	conf *model.TaskConfig,
	stop chan bool) error {
	location, err := conf.ProjectRef.Location()

	if err != nil {
		return err
	}

	gitCommands := []string{
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("rm -rf %v", self.Directory),
		fmt.Sprintf("git clone %v '%v'", location, self.Directory),
		fmt.Sprintf("cd %v; git checkout %v", self.Directory, conf.Task.Revision),
	}

	cmdsJoined := strings.Join(gitCommands, "\n")

	fetchSourceCmd := &command.LocalCommand{
		CmdString:        cmdsJoined,
		WorkingDirectory: conf.WorkDir,
		Stdout:           pluginLogger.GetTaskLogWriter(slogger.INFO),
		Stderr:           pluginLogger.GetTaskLogWriter(slogger.ERROR),
		ScriptMode:       true,
	}

	pluginLogger.LogExecution(slogger.INFO, "Fetching source from git...")
	if err := fetchSourceCmd.Run(); err != nil {
		return err
	}
	pluginLogger.Flush()

	// Fetch source for the modules
	for _, moduleName := range conf.BuildVariant.Modules {
		pluginLogger.LogExecution(slogger.INFO, "Fetching module: %v", moduleName)
		module, err := conf.Project.GetModuleByName(moduleName)
		if err != nil {
			pluginLogger.LogExecution(slogger.ERROR, "Couldn't get module %v: %v",
				moduleName, err)
			continue
		}
		if module == nil {
			pluginLogger.LogExecution(slogger.ERROR, "No module found for %v",
				moduleName)
			continue
		}

		moduleBase := filepath.Join(module.Prefix, module.Name)
		moduleDir := filepath.Join(conf.WorkDir, moduleBase, "/_")

		err = os.MkdirAll(moduleDir, 0755)
		if err != nil {
			return err
		}
		// clear the destination
		err = os.RemoveAll(moduleDir)
		if err != nil {
			return err
		}

		moduleCmds := []string{
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("set -o verbose"),
			fmt.Sprintf("git clone %v '%v'", module.Repo, filepath.ToSlash(moduleBase)),
			fmt.Sprintf("cd %v; git checkout '%v'", filepath.ToSlash(moduleBase), module.Branch),
		}

		moduleFetchCmd := &command.LocalCommand{
			CmdString:        strings.Join(moduleCmds, "\n"),
			WorkingDirectory: filepath.ToSlash(filepath.Join(conf.WorkDir, self.Directory)),
			Stdout:           pluginLogger.GetTaskLogWriter(slogger.INFO),
			Stderr:           pluginLogger.GetTaskLogWriter(slogger.ERROR),
			ScriptMode:       true,
		}
		err = moduleFetchCmd.Run()
		if err != nil {
			return err
		}
		pluginLogger.Flush()
	}

	return nil

}
