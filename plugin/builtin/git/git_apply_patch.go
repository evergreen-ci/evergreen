package git

import (
	"10gen.com/mci"
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/mitchellh/mapstructure"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// GitApplyPatchCommand is a command to pull a patch from the api server
// and apply it to the given directory using `git apply`. If there are
// module patch sets included in the patch, those are applied to their
// proper directory as well.
type GitApplyPatchCommand struct {
	//The root directory (locally) that the code should be checked out into.
	//Must be a valid non-blank directory name.
	Directory string
}

func (self *GitApplyPatchCommand) Name() string {
	return ApplyPatchCmdName
}

// ParseParams reads the command's configuration and returns any errors that occur.
func (self *GitApplyPatchCommand) ParseParams(params map[string]interface{}) error {
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

// Execute pulls the task's patch and then applies it
func (self *GitApplyPatchCommand) Execute(pluginLogger plugin.PluginLogger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {

	//Apply patches only if necessary
	if conf.Task.Requester == mci.PatchVersionRequester {
		pluginLogger.LogExecution(slogger.INFO, "Fetching patch.")
		patch, err := self.GetPatch(conf, pluginCom, pluginLogger)
		if err != nil {
			pluginLogger.LogExecution(slogger.ERROR, "Failed to get patch: %v", err)
			return fmt.Errorf("Failed to get patch: %v", err)
		}

		err = self.applyPatch(conf, patch, pluginLogger)
		if err != nil {
			pluginLogger.LogExecution(slogger.INFO, "Failed to apply patch: %v", err)
			return fmt.Errorf("Failed to apply patch: %v", err)
		}
	}
	return nil
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure.
func (self GitApplyPatchCommand) GetPatch(conf *model.TaskConfig,
	pluginCom plugin.PluginCommunicator, pluginLogger plugin.PluginLogger) (*model.Patch, error) {
	patch := &model.Patch{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := pluginCom.TaskGetJSON(GitPatchPath)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				pluginLogger.LogExecution(slogger.WARN,
					"Error connecting to API server: %v", err)
				return util.RetriableError{err}
			}
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				//nothing broke, but no patch was found for task Id - no retry
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					pluginLogger.LogExecution(slogger.ERROR, "Error reading response body")
				}
				msg := fmt.Sprintf("no patch found for task: %v", string(body))
				pluginLogger.LogExecution(slogger.WARN, msg)
				return fmt.Errorf(msg)
			}
			if resp != nil && resp.StatusCode == http.StatusInternalServerError {
				//something went wrong in api server
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					pluginLogger.LogExecution(slogger.ERROR, "Error reading response body")
				}
				msg := fmt.Sprintf("error fetching patch from server: %v", string(body))
				pluginLogger.LogExecution(slogger.WARN, msg)
				return util.RetriableError{
					fmt.Errorf(msg),
				}
			}
			if resp != nil && resp.StatusCode == http.StatusConflict {
				//wrong secret
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					pluginLogger.LogExecution(slogger.ERROR, "Error reading response body")
				}
				msg := fmt.Sprintf("secret conflict: %v", string(body))
				pluginLogger.LogExecution(slogger.ERROR, msg)
				return fmt.Errorf(msg)
			}
			if resp == nil {
				pluginLogger.LogExecution(slogger.WARN, "Empty response from API server")
				return util.RetriableError{fmt.Errorf("empty response")}
			} else {
				err = util.ReadJSONInto(resp.Body, patch)
				if err != nil {
					pluginLogger.LogExecution(slogger.ERROR,
						"Error reading json into patch struct: %v", err)
					return util.RetriableError{err}
				}
				return nil
			}
		},
	)

	retryFail, err := util.RetryArithmeticBackoff(retriableGet, 5, 5*time.Second)
	if retryFail {
		return nil, fmt.Errorf("getting patch failed after %v tries: %v", 10, err)
	}
	if err != nil {
		return nil, fmt.Errorf("getting patch failed: %v", err)
	}
	return patch, nil
}

// applyPatch is used by the agent to copy patch data onto disk
// and then call the necessary git commands to apply the patch file
func (self *GitApplyPatchCommand) applyPatch(conf *model.TaskConfig,
	patch *model.Patch, pluginLogger plugin.PluginLogger) error {

	// patch sets and contain multiple patches, some of them for modules
	for _, patchPart := range patch.Patches {
		var dir string
		if patchPart.ModuleName == "" {
			// if patch is not part of a module, just apply patch against src root
			dir = self.Directory
			pluginLogger.LogExecution(slogger.INFO, "Applying patch with git...")
		} else {
			// if patch is part of a module, apply patch in module root
			module, err := conf.Project.GetModuleByName(patchPart.ModuleName)
			if err != nil {
				return fmt.Errorf("Error getting module: %v", err)
			}
			if module == nil {
				return fmt.Errorf("Module not found: %v", patchPart.ModuleName)
			}

			// skip the module if this build variant does not use it
			if !util.SliceContains(conf.BuildVariant.Modules, module.Name) {
				pluginLogger.LogExecution(slogger.INFO, "Skipping patch for"+
					" module %v, since the current build variant does not"+
					" use it", module.Name)
				continue
			}

			dir = filepath.Join(self.Directory, module.Prefix, module.Name)
			pluginLogger.LogExecution(slogger.INFO, "Applying module patch with git...")
		}

		// create a temporary folder and store patch files on disk,
		// for later use in shell script
		tempFile, err := ioutil.TempFile("", "mcipatch_")
		if err != nil {
			return err
		}
		defer tempFile.Close()
		_, err = io.WriteString(tempFile, patchPart.PatchSet.Patch)
		if err != nil {
			return err
		}
		tempAbsPath := tempFile.Name()

		// this applies the patch using the patch files in the temp directory
		patchCommandStrings := []string{
			fmt.Sprintf("set -o verbose"),
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("ls"),
			fmt.Sprintf("cd '%v'", dir),
			fmt.Sprintf("git checkout '%v'", patchPart.Githash),
			fmt.Sprintf("git apply --check --whitespace=fix '%v'", tempAbsPath),
			fmt.Sprintf("git apply --stat '%v'", tempAbsPath),
			fmt.Sprintf("git apply --whitespace=fix < '%v'", tempAbsPath),
		}

		cmdsJoined := strings.Join(patchCommandStrings, "\n")
		patchCmd := &command.LocalCommand{
			CmdString:        cmdsJoined,
			WorkingDirectory: conf.WorkDir,
			Stdout:           pluginLogger.GetTaskLogWriter(slogger.INFO),
			Stderr:           pluginLogger.GetTaskLogWriter(slogger.ERROR),
			ScriptMode:       true,
		}

		err = patchCmd.Run()
		if err != nil {
			return err
		}
		pluginLogger.Flush()
	}
	return nil
}

// servePatch is the API hook for returning patch data as json
func servePatch(w http.ResponseWriter, r *http.Request) {
	task := plugin.GetTask(r)
	patch, err := task.FetchPatch()
	if err != nil {
		msg := fmt.Sprintf("error fetching patch for task %v from db: %v", task.Id, err)
		mci.Logger.Logf(slogger.ERROR, msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if patch == nil {
		msg := fmt.Sprintf("no patch found for task %v", task.Id)
		mci.Logger.Errorf(slogger.ERROR, msg)
		http.Error(w, msg, http.StatusNotFound)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, patch)
}
