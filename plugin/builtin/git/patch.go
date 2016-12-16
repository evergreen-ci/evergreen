package git

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
)

// GitApplyPatchCommand is deprecated. Its functionality is now a part of GitGetProjectCommand.
type GitApplyPatchCommand struct{}

func (*GitApplyPatchCommand) Name() string                                    { return ApplyPatchCmdName }
func (*GitApplyPatchCommand) Plugin() string                                  { return GitPluginName }
func (*GitApplyPatchCommand) ParseParams(params map[string]interface{}) error { return nil }
func (*GitApplyPatchCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	pluginLogger.LogExecution(slogger.INFO,
		"WARNING: git.apply_patch is deprecated. Patches are applied in git.get_project ")
	return nil
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure.
func (ggpc GitGetProjectCommand) GetPatch(conf *model.TaskConfig,
	pluginCom plugin.PluginCommunicator, pluginLogger plugin.Logger) (*patch.Patch, error) {
	patch := &patch.Patch{}
	retriableGet := util.RetriableFunc(
		func() error {
			resp, err := pluginCom.TaskGetJSON(GitPatchPath)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				//Some generic error trying to connect - try again
				pluginLogger.LogExecution(slogger.WARN, "Error connecting to API server: %v", err)
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

// getPatchContents() dereferences any patch files that are stored externally, fetching them from
// the API server, and setting them into the patch object.
func (ggpc GitGetProjectCommand) getPatchContents(conf *model.TaskConfig, com plugin.PluginCommunicator, log plugin.Logger, p *patch.Patch) error {
	for i, patchPart := range p.Patches {
		// If the patch isn't stored externally, no need to do anything.
		if patchPart.PatchSet.PatchFileId == "" {
			continue
		}
		// otherwise, fetch the contents and load it into the patch object
		log.LogExecution(slogger.INFO, "Fetching patch contents for %v", patchPart.PatchSet.PatchFileId)
		var result []byte
		retriableGet := util.RetriableFunc(
			func() error {
				resp, err := com.TaskGetJSON(fmt.Sprintf("%s/%s", GitPatchFilePath, patchPart.PatchSet.PatchFileId))
				if resp != nil {
					defer resp.Body.Close()
				}
				if err != nil {
					//Some generic error trying to connect - try again
					log.LogExecution(slogger.WARN, "Error connecting to API server: %v", err)
					return util.RetriableError{err}
				}
				if resp != nil && resp.StatusCode != http.StatusOK {
					log.LogExecution(slogger.WARN, "Unexpected status code %v, retrying", resp.StatusCode)
					resp.Body.Close()
					return util.RetriableError{fmt.Errorf("Unexpected status code %v", resp.StatusCode)}
				}
				result, err = ioutil.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				return nil
			})

		_, err := util.RetryArithmeticBackoff(retriableGet, 5, 5*time.Second)
		if err != nil {
			return err
		}
		p.Patches[i].PatchSet.Patch = string(result)
	}
	return nil
}

// GetPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed. If the patch is empty it will not apply the patch.
func GetPatchCommands(modulePatch patch.ModulePatch, dir, patchPath string) []string {
	patchCommands := []string{
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("ls"),
		fmt.Sprintf("cd '%v'", dir),
		fmt.Sprintf("git checkout '%v'", modulePatch.Githash),
	}
	if modulePatch.PatchSet.Patch == "" {
		return patchCommands
	}
	return append(patchCommands, []string{
		fmt.Sprintf("git apply --check --whitespace=fix '%v'", patchPath),
		fmt.Sprintf("git apply --stat '%v'", patchPath),
		fmt.Sprintf("git apply --whitespace=fix < '%v'", patchPath),
	}...)
}

// applyPatch is used by the agent to copy patch data onto disk
// and then call the necessary git commands to apply the patch file
func (ggpc *GitGetProjectCommand) applyPatch(conf *model.TaskConfig,
	p *patch.Patch, pluginLogger plugin.Logger) error {
	// patch sets and contain multiple patches, some of them for modules
	for _, patchPart := range p.Patches {
		var dir string
		if patchPart.ModuleName == "" {
			// if patch is not part of a module, just apply patch against src root
			dir = ggpc.Directory
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

			dir = filepath.Join(ggpc.Directory, module.Prefix, module.Name)
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
		patchCommandStrings := GetPatchCommands(patchPart, dir, tempAbsPath)
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
	patch, err := patch.FindOne(patch.ByVersion(task.Version))
	if err != nil {
		msg := fmt.Sprintf("error fetching patch for task %v from db: %v", task.Id, err)
		evergreen.Logger.Logf(slogger.ERROR, msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	if patch == nil {
		msg := fmt.Sprintf("no patch found for task %v", task.Id)
		evergreen.Logger.Errorf(slogger.ERROR, msg)
		http.Error(w, msg, http.StatusNotFound)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, patch)
}

// servePatchFile is the API hook for returning raw patch contents
func servePatchFile(w http.ResponseWriter, r *http.Request) {
	fileId := mux.Vars(r)["patchfile_id"]
	data, err := db.GetGridFile(patch.GridFSPrefix, fileId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file from db: %v", err), http.StatusInternalServerError)
		return
	}
	defer data.Close()
	io.Copy(w, data)
}
