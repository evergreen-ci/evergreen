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
	"github.com/evergreen-ci/evergreen/thirdparty"
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

	ProjectToken string `plugin:"expand" mapstructure:"token"`

	base
}

type cloneOpts struct {
	location *url.URL
	owner    string
	repo     string
	branch   string
	dir      string
	token    string
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

	return nil
}

func buildHTTPCloneCommand(opts cloneOpts) ([]string, error) {
	clone := fmt.Sprintf("git clone https://%s@%s/%s/%s.git '%s'", opts.token, opts.location.Host, opts.owner, opts.repo, opts.dir)

	if opts.branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, opts.branch)
	}

	redactedClone := strings.Replace(clone, opts.token, "[redacted oauth token]", -1)
	return []string{
		"set +o xtrace",
		fmt.Sprintf(`echo %s`, strconv.Quote(redactedClone)),
		clone,
		"set -o xtrace",
		fmt.Sprintf("cd %s", opts.dir),
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

func (c *gitFetchProject) buildCloneCommand(conf *model.TaskConfig, oauthToken string) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf("rm -rf %s", c.Directory),
	}

	var cloneCmd []string
	if oauthToken == "" {
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
		opts := cloneOpts{
			location: location,
			owner:    conf.ProjectRef.Owner,
			repo:     conf.ProjectRef.Repo,
			branch:   conf.ProjectRef.Branch,
			dir:      c.Directory,
			token:    oauthToken,
		}
		cloneCmd, err = buildHTTPCloneCommand(opts)
		if err != nil {
			return nil, err
		}
	}

	gitCommands = append(gitCommands, cloneCmd...)

	if conf.GithubPatchData.PRNumber != 0 {
		var ref, commitToTest, branchName string
		if conf.Task.IsMergeRequest() {
			// GitHub creates a ref at refs/pull/[pr number]/merge
			// pointing to the test merge commit they generate
			// See: https://developer.github.com/v3/git/#checking-mergeability-of-pull-requests
			// and: https://docs.travis-ci.com/user/pull-requests/#my-pull-request-isnt-being-built
			ref = "merge"
			commitToTest = conf.GithubPatchData.MergeCommitSHA
			branchName = fmt.Sprintf("evg-merge-test-%s", util.RandomString())
		} else {
			// Github creates a ref called refs/pull/[pr number]/head
			// that provides the entire tree of changes, including merges
			ref = "head"
			commitToTest = conf.GithubPatchData.HeadHash
			branchName = fmt.Sprintf("evg-pr-test-%s", util.RandomString())
		}
		gitCommands = append(gitCommands, []string{
			fmt.Sprintf(`git fetch origin "pull/%d/%s:%s"`, conf.GithubPatchData.PRNumber, ref, branchName),
			fmt.Sprintf(`git checkout "%s"`, branchName),
			fmt.Sprintf("git reset --hard %s", commitToTest),
		}...)

	} else {
		gitCommands = append(gitCommands,
			fmt.Sprintf("git reset --hard %s", conf.Task.Revision))
	}

	return gitCommands, nil
}

func (c *gitFetchProject) buildModuleCloneCommand(cloneURI, owner, repo, moduleBase, ref, oauthToken, requester string, modulePatch *patch.ModulePatch) ([]string, error) {

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
		opts := cloneOpts{
			location: url,
			owner:    owner,
			repo:     repo,
			dir:      moduleBase,
			token:    oauthToken,
		}
		cmds, err := buildHTTPCloneCommand(opts)
		if err != nil {
			return nil, err
		}
		gitCommands = append(gitCommands, cmds...)
	}

	if modulePatchProvidedForMergeTest(requester, modulePatch) {
		branchName := fmt.Sprintf("evg-merge-test-%s", util.RandomString())
		gitCommands = append(gitCommands,
			fmt.Sprintf(`git fetch origin "pull/%s/merge:%s"`, modulePatch.PatchSet.Patch, branchName),
			fmt.Sprintf("git checkout '%s'", branchName),
			fmt.Sprintf("git reset --hard %s", modulePatch.Githash),
		)
	} else {
		gitCommands = append(gitCommands, fmt.Sprintf("git checkout '%s'", ref))
	}

	return gitCommands, nil
}

// Execute gets the source code required by the project
func (c *gitFetchProject) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	var err error

	// expand the github parameters before running the task
	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding github parameters")
	}
	// c.ProjectToken (from the project's YAML file) takes precedence
	// over db.admin.find({"_id": "global"},{"credentials.github": 1}), unless it is an empty string.
	oauthToken := conf.Expansions.Get("global_github_oauth_token")
	if len(c.ProjectToken) != 0 {
		oauthToken = c.ProjectToken
	}

	if strings.HasPrefix(oauthToken, "token") {
		splitToken := strings.Split(oauthToken, " ")
		if len(splitToken) != 2 {
			return errors.New("token format is invalid")
		}
		oauthToken = splitToken[1]
	}

	gitCommands, err := c.buildCloneCommand(conf, oauthToken)
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
	if oauthToken != "" {
		redactedCmds = strings.Replace(redactedCmds, oauthToken, "[redacted oauth token]", -1)
	}
	logger.Execution().Debug(fmt.Sprintf("Commands are: %s", redactedCmds))

	if err = fetchSourceCmd.Run(ctx); err != nil {
		return errors.Wrap(err, "problem running fetch command")
	}

	var p *patch.Patch
	if evergreen.IsPatchRequester(conf.Task.Requester) {
		logger.Execution().Info("Fetching patch.")
		p, err = comm.GetTaskPatch(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
		if err != nil {
			return errors.Wrap(err, "Failed to get patch")
		}
	}

	// Fetch source for the modules
	for _, moduleName := range conf.BuildVariant.Modules {
		if ctx.Err() != nil {
			return errors.New("git.get_project command aborted while applying modules")
		}
		logger.Execution().Infof("Fetching module: %s", moduleName)

		var module *model.Module
		module, err = conf.Project.GetModuleByName(moduleName)
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
		var owner, repo string
		owner, repo, err = thirdparty.ParseGitUrl(module.Repo)
		if err != nil {
			logger.Execution().Error(err.Error())
		}
		if owner == "" || repo == "" {
			continue
		}

		var modulePatch *patch.ModulePatch
		if p != nil {
			// find module among the patch's Patches
			for i := range p.Patches {
				if p.Patches[i].ModuleName == moduleName {
					modulePatch = &p.Patches[i]
					break
				}
			}
		}

		var moduleCmds []string

		moduleCmds, err = c.buildModuleCloneCommand(module.Repo, owner, repo, moduleBase, revision, oauthToken, conf.Task.Requester, modulePatch)
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
	if conf.Task.Requester == evergreen.PatchVersionRequester {
		if err = c.getPatchContents(ctx, comm, logger, conf, p); err != nil {
			err = errors.Wrap(err, "Failed to get patch contents")
			logger.Execution().Errorf(err.Error())
			return err
		}

		if err = c.applyPatch(ctx, logger, conf, p); err != nil {
			err = errors.Wrap(err, "Failed to apply patch")
			logger.Execution().Infof(err.Error())
			return err
		}
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

	return fmt.Sprintf("git apply --binary --index < '%s'", patchFile), nil
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
		}

		if len(patchPart.PatchSet.Patch) == 0 {
			logger.Execution().Info("Skipping empty patch file...")
			continue

		} else if patchPart.ModuleName == "" {
			logger.Execution().Info("Applying patch with git...")

		} else {
			logger.Execution().Info("Applying module patch with git...")
		}

		// create a temporary folder and store patch files on disk,
		// for later use in shell script
		tempFile, err := ioutil.TempFile("", "mcipatch_")
		if err != nil {
			return errors.WithStack(err)
		}
		defer tempFile.Close() //nolint: evg
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

func modulePatchProvidedForMergeTest(requester string, modulePatch *patch.ModulePatch) bool {
	isMergeTest := requester == evergreen.MergeTestRequester
	patchProvided := (modulePatch != nil) && (modulePatch.PatchSet.Patch != "")

	return isMergeTest && patchProvided
}
