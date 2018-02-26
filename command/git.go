package command

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

	Token string `plugin:"expand"`

	base
}

func gitFetchProjectFactory() Command   { return &gitFetchProject{} }
func (c *gitFetchProject) Name() string { return "git.get_project" }

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

	if strings.HasPrefix(c.Token, "token") {
		splitToken := strings.Split(c.Token, " ")
		if len(splitToken) != 2 {
			return errors.New("token format is invalid")
		}
		c.Token = splitToken[1]
	}
	return nil
}

func buildHTTPCloneCommand(location *url.URL, branch, dir, token string) ([]string, error) {
	location.Scheme = "https"

	tokenFlag := ""
	if token != "" {
		if location.Host != "github.com" {
			return nil, errors.Errorf("Token support is only for Github, refusing to send token to '%s'", location.Host)
		}
		tokenFlag = fmt.Sprintf("-c 'credential.%s://%s.username=%s'", location.Scheme, location.Host, token)
	}

	clone := fmt.Sprintf("GIT_ASKPASS='true' git %s clone '%s' '%s'", tokenFlag, location.String(), dir)

	if branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, branch)
	}

	redactedClone := clone
	if tokenFlag != "" {
		redactedClone = strings.Replace(clone, tokenFlag, "-c '[redacted oauth token]'", -1)
	}
	return []string{
		"set +o xtrace",
		fmt.Sprintf(`echo %s`, strconv.Quote(redactedClone)),
		clone,
		"set -o xtrace",
		fmt.Sprintf("cd %s", dir),
	}, nil
}

func buildSSHCloneCommand(location, branch, dir string) ([]string, error) {
	cloneCmd := fmt.Sprintf("git clone '%s' '%s'", location, dir)
	if branch != "" {
		cloneCmd = fmt.Sprintf("%s --branch '%s'", cloneCmd, branch)
	}

	return []string{
		cloneCmd,
		fmt.Sprintf("cd %s", dir),
	}, nil
}

func (c *gitFetchProject) buildCloneCommand(conf *model.TaskConfig) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf("rm -rf %s", c.Directory),
	}

	var cloneCmd []string
	if c.Token == "" {
		location, err := conf.ProjectRef.Location()
		if err != nil {
			return nil, err
		}
		cloneCmd, err = buildSSHCloneCommand(location, conf.ProjectRef.Branch, c.Directory)
		if err != nil {
			return nil, err
		}

	} else {
		location, err := conf.ProjectRef.HTTPLocation()
		if err != nil {
			return nil, err
		}
		cloneCmd, err = buildHTTPCloneCommand(location, conf.ProjectRef.Branch, c.Directory, c.Token)
		if err != nil {
			return nil, err
		}
	}

	gitCommands = append(gitCommands, cloneCmd...)

	if conf.GithubPatchData.PRNumber != 0 {
		branchName := fmt.Sprintf("evg-pr-test-%s", util.RandomString())

		gitCommands = append(gitCommands, []string{
			// Github creates a ref called refs/pull/[pr number]/head
			// that provides the entire tree of changes, including merges
			fmt.Sprintf(`git fetch origin "pull/%d/head:%s"`, conf.GithubPatchData.PRNumber, branchName),
			fmt.Sprintf(`git checkout "%s"`, branchName),
			fmt.Sprintf("git reset --hard %s", conf.GithubPatchData.HeadHash),
		}...)

	} else {
		gitCommands = append(gitCommands,
			fmt.Sprintf("git reset --hard %s", conf.Task.Revision))
	}

	return gitCommands, nil
}

func (c *gitFetchProject) buildModuleCloneCommand(cloneURI, moduleBase, ref string) ([]string, error) {
	if cloneURI == "" {
		return nil, errors.New("empty repository URI")
	}
	if moduleBase == "" {
		return nil, errors.New("empty clone path")
	}
	if ref == "" {
		return nil, errors.New("empty ref/branch to checkout")
	}
	moduleBase = filepath.ToSlash(moduleBase)

	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
	}

	if strings.Contains(cloneURI, "git@github.com:") {
		cmds, err := buildSSHCloneCommand(cloneURI, "", moduleBase)
		if err != nil {
			return nil, err
		}
		gitCommands = append(gitCommands, cmds...)

	} else {
		url, err := url.Parse(cloneURI)
		if err != nil {
			return nil, errors.Wrap(err, "repository URL is invalid")
		}
		cmds, err := buildHTTPCloneCommand(url, "", moduleBase, c.Token)
		if err != nil {
			return nil, err
		}
		gitCommands = append(gitCommands, cmds...)
	}
	gitCommands = append(gitCommands, fmt.Sprintf("git checkout '%s'", ref))

	return gitCommands, nil
}

// Execute gets the source code required by the project
func (c *gitFetchProject) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	var err error

	// expand the github parameters before running the task
	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return err
	}

	gitCommands, err := c.buildCloneCommand(conf)
	if err != nil {
		return errors.WithStack(err)
	}

	cmdsJoined := strings.Join(gitCommands, "\n")

	stdOut := logger.TaskWriter(level.Info)
	defer stdOut.Close()
	stdErr := logger.TaskWriter(level.Error)
	defer stdErr.Close()
	output := subprocess.OutputOptions{Output: stdOut, Error: stdErr}
	fetchSourceCmd := subprocess.NewLocalCommand(cmdsJoined, conf.WorkDir, "bash", nil, true)
	if err = fetchSourceCmd.SetOutput(output); err != nil {
		return errors.WithStack(err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger.Execution().Info("Fetching source from git...")
	redactedCmds := cmdsJoined
	if c.Token != "" {
		redactedCmds = strings.Replace(redactedCmds, c.Token, "[redacted oauth token]", -1)
	}
	logger.Execution().Debug(fmt.Sprintf("Commands are: %s", redactedCmds))

	if err = fetchSourceCmd.Run(ctx); err != nil {
		return errors.Wrap(err, "problem running fetch command")
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
		revision := c.Revisions[moduleName]

		// if there is no revision, then use the revision from the module, then branch name
		if revision == "" {
			if module.Ref != "" {
				revision = module.Ref
			} else {
				revision = module.Branch
			}
		}

		moduleCmds, err := c.buildModuleCloneCommand(module.Repo, moduleBase, revision)
		if err != nil {
			return err
		}

		moduleFetchCmd := subprocess.NewLocalCommand(
			strings.Join(moduleCmds, "\n"),
			filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory)),
			"bash",
			nil,
			true)

		if err = moduleFetchCmd.SetOutput(output); err != nil {
			return err
		}

		if err = moduleFetchCmd.Run(ctx); err != nil {
			return errors.Wrap(err, "problem with git command")
		}
	}

	//Apply patches if necessary
	if conf.Task.Requester != evergreen.PatchVersionRequester {
		return nil
	}

	logger.Execution().Info("Fetching patch.")
	patch, err := comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
	if err != nil {
		err = errors.Wrap(err, "Failed to get patch")
		logger.Execution().Error(err.Error())
		return err
	}

	if err = c.getPatchContents(ctx, comm, logger, conf, patch); err != nil {
		err = errors.Wrap(err, "Failed to get patch contents")
		logger.Execution().Errorf(err.Error())
		return err
	}

	if err = c.applyPatch(ctx, logger, conf, patch); err != nil {
		err = errors.Wrap(err, "Failed to apply patch")
		logger.Execution().Infof(err.Error())
		return err
	}

	return nil
}

// getPatchContents() dereferences any patch files that are stored externally, fetching them from
// the API server, and setting them into the patch object.
func (c *gitFetchProject) getPatchContents(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig, patch *patch.Patch) error {

	if patch == nil {
		return errors.New("cannot get patch contents for nil patch")
	}

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

		patch.Patches[i].PatchSet.Patch = result
	}
	return nil
}

// isMailboxPatch checks if the first line of a patch file
// has "From ". If so, it's assumed to be a mailbox-style patch, otherwise
// it's a diff
func isMailboxPatch(patchFile string) (bool, error) {
	file, err := os.Open(patchFile)
	if err != nil {
		return false, errors.Wrap(err, "failed to read patch file")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err = scanner.Err(); err != nil {
			return false, errors.Wrap(err, "failed to read patch file")
		}

		// otherwise, it's EOF. Empty patches are not errors!
		return false, nil
	}
	line := scanner.Text()

	return strings.HasPrefix(line, "From "), nil
}

// getApplyCommand determines the patch type. If the patch is a mailbox-style
// patch, it uses git-am (see https://git-scm.com/docs/git-am), otherwise
// it uses git apply
func getApplyCommand(patchFile string) (string, error) {
	isMBP, err := isMailboxPatch(patchFile)
	if err != nil {
		return "", errors.Wrap(err, "can't check patch type")
	}

	if isMBP {
		return fmt.Sprintf(`git -c "user.name=Evergreen Agent" -c "user.email=no-reply@evergreen.mongodb.com" am < '%s'`, patchFile), nil
	}

	return fmt.Sprintf("git apply --binary --whitespace=fix --index < '%s'", patchFile), nil
}

// getPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed, except for apply. If the patch is empty it will not apply the patch.
func getPatchCommands(modulePatch patch.ModulePatch, dir, patchPath string) []string {
	patchCommands := []string{
		fmt.Sprintf("set -o xtrace"),
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("ls"),
		fmt.Sprintf("cd '%s'", dir),
		fmt.Sprintf("git reset --hard '%s'", modulePatch.Githash),
	}
	if modulePatch.PatchSet.Patch == "" {
		return patchCommands
	}
	return append(patchCommands, []string{
		fmt.Sprintf("git apply --stat '%v' || true", patchPath),
	}...)
}

// applyPatch is used by the agent to copy patch data onto disk
// and then call the necessary git commands to apply the patch file
func (c *gitFetchProject) applyPatch(ctx context.Context, logger client.LoggerProducer,
	conf *model.TaskConfig, p *patch.Patch) error {

	stdOut := logger.TaskWriter(level.Info)
	defer stdOut.Close()
	stdErr := logger.TaskWriter(level.Error)
	defer stdErr.Close()

	output := subprocess.OutputOptions{Output: stdOut, Error: stdErr}

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
			if !util.StringSliceContains(conf.BuildVariant.Modules, module.Name) {
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
		applyCommand, err := getApplyCommand(tempAbsPath)
		if err != nil {
			logger.Execution().Error("Could not to determine patch type")
			return errors.WithStack(err)
		}
		patchCommandStrings = append(patchCommandStrings, applyCommand)
		cmdsJoined := strings.Join(patchCommandStrings, "\n")

		patchCmd := subprocess.NewLocalCommand(cmdsJoined, conf.WorkDir, "bash", nil, true)
		if err = patchCmd.SetOutput(output); err != nil {
			return errors.WithStack(err)
		}

		if err = patchCmd.Run(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
