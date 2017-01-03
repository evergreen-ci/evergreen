package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/mitchellh/mapstructure"
	"github.com/tychoish/grip/slogger"
)

// GitGetProjectCommand is a command that fetches source code from git for the project
// associated with the current task
type GitGetProjectCommand struct {
	// The root directory (locally) that the code should be checked out into.
	// Must be a valid non-blank directory name.
	Directory string

	// Revisions are the optional revisions associated with the modules of a project.
	// Note: If a module does not have a revision it will use the module's branch to get the project.
	Revisions map[string]string
}

func (ggpc *GitGetProjectCommand) Name() string {
	return GetProjectCmdName
}

func (ggpc *GitGetProjectCommand) Plugin() string {
	return GitPluginName
}

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (ggpc *GitGetProjectCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, ggpc)
	if err != nil {
		return err
	}

	if ggpc.Directory == "" {
		return fmt.Errorf("error parsing '%v' params: value for directory "+
			"must not be blank", ggpc.Name())
	}
	return nil
}

// Execute gets the source code required by the project
func (ggpc *GitGetProjectCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator,
	conf *model.TaskConfig,
	stop chan bool) error {

	// expand the github parameters before running the task
	if err := plugin.ExpandValues(&ggpc.Revisions, conf.Expansions); err != nil {
		return err
	}

	location, err := conf.ProjectRef.Location()

	if err != nil {
		return err
	}

	gitCommands := []string{
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("rm -rf %v", ggpc.Directory),
		fmt.Sprintf("git clone %v '%v'", location, ggpc.Directory),
		fmt.Sprintf("cd %v; git checkout %v", ggpc.Directory, conf.Task.Revision),
	}

	cmdsJoined := strings.Join(gitCommands, "\n")

	fetchSourceCmd := &command.LocalCommand{
		CmdString:        cmdsJoined,
		WorkingDirectory: conf.WorkDir,
		Stdout:           pluginLogger.GetTaskLogWriter(slogger.INFO),
		Stderr:           pluginLogger.GetTaskLogWriter(slogger.ERROR),
		ScriptMode:       true,
	}

	errChan := make(chan error)
	go func() {
		pluginLogger.LogExecution(slogger.INFO, "Fetching source from git...")
		errChan <- fetchSourceCmd.Run()
		pluginLogger.Flush()
	}()

	// wait until the command finishes or the stop channel is tripped
	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Got kill signal")
		if fetchSourceCmd.Cmd != nil {
			pluginLogger.LogExecution(slogger.INFO, "Stopping process: %v", fetchSourceCmd.Cmd.Process.Pid)
			if err := fetchSourceCmd.Stop(); err != nil {
				pluginLogger.LogExecution(slogger.ERROR, "Error occurred stopping process: %v", err)
			}
		}
		return fmt.Errorf("Fetch command interrupted.")
	}

	// Fetch source for the modules
	for _, moduleName := range conf.BuildVariant.Modules {
		pluginLogger.LogExecution(slogger.INFO, "Fetching module: %v", moduleName)
		module, err := conf.Project.GetModuleByName(moduleName)
		if err != nil {
			pluginLogger.LogExecution(slogger.ERROR, "Couldn't get module %v: %v", moduleName, err)
			continue
		}
		if module == nil {
			pluginLogger.LogExecution(slogger.ERROR, "No module found for %v", moduleName)
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

		revision := ggpc.Revisions[moduleName]

		// if there is no revision, then use the branch name
		if revision == "" {
			revision = module.Branch
		}

		moduleCmds := []string{
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("set -o verbose"),
			fmt.Sprintf("git clone %v '%v'", module.Repo, filepath.ToSlash(moduleBase)),
			fmt.Sprintf("cd %v; git checkout '%v'", filepath.ToSlash(moduleBase), revision),
		}

		moduleFetchCmd := &command.LocalCommand{
			CmdString:        strings.Join(moduleCmds, "\n"),
			WorkingDirectory: filepath.ToSlash(filepath.Join(conf.WorkDir, ggpc.Directory)),
			Stdout:           pluginLogger.GetTaskLogWriter(slogger.INFO),
			Stderr:           pluginLogger.GetTaskLogWriter(slogger.ERROR),
			ScriptMode:       true,
		}

		go func() {
			errChan <- moduleFetchCmd.Run()
			pluginLogger.Flush()
		}()

		// wait until the command finishes or the stop channel is tripped
		select {
		case err := <-errChan:
			if err != nil {
				return err
			}
		case <-stop:
			pluginLogger.LogExecution(slogger.INFO, "Got kill signal")
			if moduleFetchCmd.Cmd != nil {
				pluginLogger.LogExecution(slogger.INFO, "Stopping process: %v", moduleFetchCmd.Cmd.Process.Pid)
				if err := moduleFetchCmd.Stop(); err != nil {
					pluginLogger.LogExecution(slogger.ERROR, "Error occurred stopping process: %v", err)
				}
			}
			return fmt.Errorf("Fetch module command interrupted.")
		}

	}

	//Apply patches if necessary
	if conf.Task.Requester != evergreen.PatchVersionRequester {
		return nil
	}
	go func() {
		pluginLogger.LogExecution(slogger.INFO, "Fetching patch.")
		patch, err := ggpc.GetPatch(conf, pluginCom, pluginLogger)
		if err != nil {
			pluginLogger.LogExecution(slogger.ERROR, "Failed to get patch: %v", err)
			errChan <- fmt.Errorf("Failed to get patch: %v", err)
		}
		err = ggpc.getPatchContents(conf, pluginCom, pluginLogger, patch)
		if err != nil {
			pluginLogger.LogExecution(slogger.ERROR, "Failed to get patch contents: %v", err)
			errChan <- fmt.Errorf("Failed to get patch contents: %v", err)
		}
		err = ggpc.applyPatch(conf, patch, pluginLogger)
		if err != nil {
			pluginLogger.LogExecution(slogger.INFO, "Failed to apply patch: %v", err)
			errChan <- fmt.Errorf("Failed to apply patch: %v", err)
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		return fmt.Errorf("Patch command interrupted.")
	}

}
