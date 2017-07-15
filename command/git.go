package command

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/level"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// gitFetchProject is a command that fetches source code from git for the project
// associated with the current task
type gitFetchProject struct {
	// The root directory (locally) that the code should be checked out into.
	// Must be a valid non-blank directory name.
	Directory string `plugin:"expand"`

	// Revisions are the optional revisions associated with the modules of a project.
	// Note: If a module does not have a revision it will use the module's branch to get the project.
	Revisions map[string]string `plugin:"expand"`
}

func gitFetchProjectFactory() Command     { return &gitFetchProject{} }
func (c *gitFetchProject) Name() string   { return "get_project" }
func (c *gitFetchProject) Plugin() string { return "git" }

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (c *gitFetchProject) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return err
	}

	if c.Directory == "" {
		return errors.Errorf("error parsing '%v' params: value for directory "+
			"must not be blank", c.Name())
	}
	return nil
}

// Execute gets the source code required by the project
func (c *gitFetchProject) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	// expand the github parameters before running the task
	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return err
	}

	location, err := conf.ProjectRef.Location()
	if err != nil {
		return err
	}

	gitCommands := []string{
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("rm -rf %s", c.Directory),
	}

	cloneCmd := fmt.Sprintf("git clone '%s' '%s'", location, c.Directory)
	if conf.ProjectRef.Branch != "" {
		cloneCmd = fmt.Sprintf("%s --branch '%s'", cloneCmd, conf.ProjectRef.Branch)
	}

	gitCommands = append(gitCommands,
		cloneCmd,
		fmt.Sprintf("cd %v; git reset --hard %s", c.Directory, conf.Task.Revision))

	cmdsJoined := strings.Join(gitCommands, "\n")

	fetchSourceCmd := &subprocess.LocalCommand{
		CmdString:        cmdsJoined,
		WorkingDirectory: conf.WorkDir,
		Stdout:           logger.TaskWriter(level.Info),
		Stderr:           logger.TaskWriter(level.Error),
		ScriptMode:       true,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errChan := make(chan error)
	go func() {
		logger.Execution().Info("Fetching source from git...")
		errChan <- fetchSourceCmd.Run(ctx)
	}()

	// wait until the command finishes or the stop channel is tripped
	select {
	case err := <-errChan:
		if err != nil {
			return errors.WithStack(err)
		}
	case <-ctx.Done():
		logger.Execution().Info("Got kill signal during git.get_project command")
		if fetchSourceCmd.Cmd != nil {
			logger.Execution().Infof("Stopping process: %d", fetchSourceCmd.Cmd.Process.Pid)
			if err := fetchSourceCmd.Stop(); err != nil {
				logger.Execution().Errorf("Error occurred stopping process: %v", err)
			}
		}
		return errors.New("Fetch command interrupted")
	}

	// Fetch source for the modules
	for _, moduleName := range conf.BuildVariant.Modules {
		if ctx.Err() != nil {
			return errors.New("git.get_project command aborted while applying modules")
		}
		logger.Execution().Infof("Fetching module: %s", moduleName)

		module, err := conf.Project.GetModuleByName(moduleName)
		if err != nil {
			logger.Execution().Errorf("Couldn't get module %s: %v", moduleName, err)
			continue
		}
		if module == nil {
			logger.Execution().Errorf("No module found for %s", moduleName)
			continue
		}

		moduleBase := filepath.Join(module.Prefix, module.Name)
		moduleDir := filepath.Join(conf.WorkDir, moduleBase, "/_")

		err = os.MkdirAll(moduleDir, 0755)
		if err != nil {
			return errors.WithStack(err)
		}
		// clear the destination
		err = os.RemoveAll(moduleDir)
		if err != nil {
			return errors.WithStack(err)
		}

		revision := c.Revisions[moduleName]

		// if there is no revision, then use the revision from the module, then branch name
		if revision == "" {
			if module.Ref != "" {
				revision = module.Ref
			} else {
				revision = module.Branch
			}
		}

		moduleCmds := []string{
			fmt.Sprintf("set -o errexit"),
			fmt.Sprintf("set -o verbose"),
			fmt.Sprintf("git clone %v '%v'", module.Repo, filepath.ToSlash(moduleBase)),
			fmt.Sprintf("cd %v; git checkout '%v'", filepath.ToSlash(moduleBase), revision),
		}

		moduleFetchCmd := &subprocess.LocalCommand{
			CmdString:        strings.Join(moduleCmds, "\n"),
			WorkingDirectory: filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory)),
			Stdout:           logger.TaskWriter(level.Info),
			Stderr:           logger.TaskWriter(level.Error),
			ScriptMode:       true,
		}

		go func() {
			errChan <- moduleFetchCmd.Run(ctx)
		}()

		// wait until the command finishes or the stop channel is tripped
		select {
		case err := <-errChan:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			logger.Execution().Info("Got kill signal")
			if moduleFetchCmd.Cmd != nil {
				logger.Execution().Infof("Stopping process: %d", moduleFetchCmd.Cmd.Process.Pid)
				if err := moduleFetchCmd.Stop(); err != nil {
					logger.Execution().Errorf("Error occurred stopping process: %v", err)
				}
			}
			return errors.New("Fetch module command interrupted")
		}
	}

	//Apply patches if necessary
	if conf.Task.Requester != evergreen.PatchVersionRequester {
		return nil
	}

	go func() {
		logger.Execution().Info("Fetching patch.")
		patch, err := comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
		if err != nil {
			logger.Execution().Errorf("Failed to get patch: %v", err)
			errChan <- errors.Wrap(err, "Failed to get patch")
		}
		err = c.getPatchContents(ctx, comm, logger, conf, patch)
		if err != nil {
			logger.Execution().Errorf("Failed to get patch contents: %v", err)
			errChan <- errors.Wrap(err, "Failed to get patch contents")
		}
		err = c.applyPatch(ctx, logger, conf, patch)
		if err != nil {
			logger.Execution().Infof("Failed to apply patch: %v", err)
			errChan <- errors.Wrap(err, "Failed to apply patch")
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		return errors.New("Patch command interrupted")
	}
}

// getPatchContents() dereferences any patch files that are stored externally, fetching them from
// the API server, and setting them into the patch object.
func (c gitFetchProject) getPatchContents(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig, patch *patch.Patch) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	for i, patchPart := range patch.Patches {
		// If the patch isn't stored externally, no need to do anything.
		if patchPart.PatchSet.PatchFileId == "" {
			continue
		}

		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		// otherwise, fetch the contents and load it into the patch object
		logger.Execution().Infof("Fetching patch contents for %s", patchPart.PatchSet.PatchFileId)

		result, err := comm.GetPatchFile(ctx, td, patchPart.PatchSet.PatchFileId)
		if err != nil {
			return errors.Wrapf(err, "problem getting patch file")
		}

		patch.Patches[i].PatchSet.Patch = string(result)
	}
	return nil
}

// getPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed. If the patch is empty it will not apply the patch.
func getPatchCommands(modulePatch patch.ModulePatch, dir, patchPath string) []string {
	patchCommands := []string{
		fmt.Sprintf("set -o verbose"),
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("ls"),
		fmt.Sprintf("cd '%s'", dir),
		fmt.Sprintf("git reset --hard '%s'", modulePatch.Githash),
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
func (c *gitFetchProject) applyPatch(ctx context.Context, logger client.LoggerProducer,
	conf *model.TaskConfig, p *patch.Patch) error {

	// patch sets and contain multiple patches, some of them for modules
	for _, patchPart := range p.Patches {
		if ctx.Err() != nil {
			return errors.New("apply patch operation canceled")
		}

		var dir string
		if patchPart.ModuleName == "" {
			// if patch is not part of a module, just apply patch against src root
			dir = c.Directory
			logger.Execution().Info("Applying patch with git...")
		} else {
			// if patch is part of a module, apply patch in module root
			module, err := conf.Project.GetModuleByName(patchPart.ModuleName)
			if err != nil {
				return errors.Wrap(err, "Error getting module")
			}
			if module == nil {
				return errors.Errorf("Module '%s' not found", patchPart.ModuleName)
			}

			// skip the module if this build variant does not use it
			if !util.SliceContains(conf.BuildVariant.Modules, module.Name) {
				logger.Execution().Infof(
					"Skipping patch for module %v: the current build variant does not use it",
					module.Name)
				continue
			}

			dir = filepath.Join(c.Directory, module.Prefix, module.Name)
			logger.Execution().Info("Applying module patch with git...")
		}

		// create a temporary folder and store patch files on disk,
		// for later use in shell script
		tempFile, err := ioutil.TempFile("", "mcipatch_")
		if err != nil {
			return errors.WithStack(err)
		}
		defer tempFile.Close()
		_, err = io.WriteString(tempFile, patchPart.PatchSet.Patch)
		if err != nil {
			return errors.WithStack(err)
		}
		tempAbsPath := tempFile.Name()

		// this applies the patch using the patch files in the temp directory
		patchCommandStrings := getPatchCommands(patchPart, dir, tempAbsPath)
		cmdsJoined := strings.Join(patchCommandStrings, "\n")
		patchCmd := &subprocess.LocalCommand{
			CmdString:        cmdsJoined,
			WorkingDirectory: conf.WorkDir,
			Stdout:           logger.TaskWriter(level.Info),
			Stderr:           logger.TaskWriter(level.Error),
			ScriptMode:       true,
		}

		if err = patchCmd.Run(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
