package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	GitFetchProjectRetries = 5
	defaultCommitterName   = "Evergreen Agent"
	defaultCommitterEmail  = "no-reply@evergreen.mongodb.com"
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

	Token string `plugin:"expand" mapstructure:"token"`

	ShallowClone bool `mapstructure:"shallow_clone"`

	RecurseSubmodules bool `mapstructure:"recurse_submodules"`

	CommitterName  string `mapstructure:"committer_name"`
	CommitterEmail string `mapstructure:"committer_email"`

	base
}

type cloneOpts struct {
	method             string
	location           string
	owner              string
	repo               string
	branch             string
	dir                string
	token              string
	shallowClone       bool
	recurseSubmodules  bool
	mergeTestRequester bool
}

func (opts cloneOpts) validate() error {
	catcher := grip.NewBasicCatcher()
	if opts.owner == "" {
		catcher.New("missing required owner")
	}
	if opts.repo == "" {
		catcher.New("missing required repo")
	}
	if opts.location == "" {
		catcher.New("missing required location")
	}
	if opts.method != "" {
		catcher.Wrap(distro.ValidateCloneMethod(opts.method), "invalid clone method")
	}
	if opts.method == distro.CloneMethodOAuth && opts.token == "" {
		catcher.New("cannot clone using OAuth if token is not set")
	}
	return catcher.Resolve()
}

func (opts cloneOpts) sshLocation() string {
	return thirdparty.FormGitUrl("github.com", opts.owner, opts.repo, "")
}

func (opts cloneOpts) httpLocation() string {
	return fmt.Sprintf("https://github.com/%s/%s.git", opts.owner, opts.repo)
}

// setLocation sets the location to clone from.
func (opts *cloneOpts) setLocation() error {
	switch opts.method {
	case "", distro.CloneMethodLegacySSH:
		opts.location = opts.sshLocation()
	case distro.CloneMethodOAuth:
		opts.location = opts.httpLocation()
	default:
		return errors.Errorf("unrecognized clone method '%s'", opts.method)
	}
	return nil
}

// getProjectMethodAndToken returns the project's clone method and token. If
// set, the project token takes precedence over global settings.
func getProjectMethodAndToken(projectToken, globalToken, globalCloneMethod string) (string, string, error) {
	if projectToken != "" {
		token, err := parseToken(projectToken)
		return distro.CloneMethodOAuth, token, err
	}
	token, err := parseToken(globalToken)
	if err != nil {
		return distro.CloneMethodLegacySSH, "", err
	}

	switch globalCloneMethod {
	// No clone method specified is equivalent to using legacy SSH.
	case "", distro.CloneMethodLegacySSH:
		return distro.CloneMethodLegacySSH, token, nil
	case distro.CloneMethodOAuth:
		if token == "" {
			return distro.CloneMethodLegacySSH, "", errors.New("cannot clone using OAuth if global token is empty")
		}
		token, err := parseToken(globalToken)
		return distro.CloneMethodOAuth, token, err
	}

	return "", "", errors.Errorf("unrecognized clone method '%s'", globalCloneMethod)
}

// parseToken parses the OAuth token, if it is in the format "token <token>";
// otherwise, it returns the token unchanged.
func parseToken(token string) (string, error) {
	if !strings.HasPrefix(token, "token") {
		return token, nil
	}
	splitToken := strings.Split(token, " ")
	if len(splitToken) != 2 {
		return "", errors.New("token format is invalid")
	}
	return splitToken[1], nil
}

func (opts cloneOpts) getCloneCommand() ([]string, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "cannot create clone command")
	}
	switch opts.method {
	case "", distro.CloneMethodLegacySSH:
		return opts.buildSSHCloneCommand()
	case distro.CloneMethodOAuth:
		return opts.buildHTTPCloneCommand()
	}
	return nil, errors.New("unrecognized clone method in options")
}

func (opts cloneOpts) buildHTTPCloneCommand() ([]string, error) {
	urlLocation, err := url.Parse(opts.location)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse URL from location")
	}
	clone := fmt.Sprintf("git clone %s '%s'", thirdparty.FormGitUrl(urlLocation.Host, opts.owner, opts.repo, opts.token), opts.dir)
	if opts.recurseSubmodules {
		clone = fmt.Sprintf("%s --recurse-submodules", clone)
	}
	if opts.shallowClone {
		// Experiments with shallow clone on AWS hosts suggest that depth 100 is as fast as 1, but 1000 is slower.
		clone = fmt.Sprintf("%s --depth 100", clone)
	}
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

func (opts cloneOpts) buildSSHCloneCommand() ([]string, error) {
	cloneCmd := fmt.Sprintf("git clone '%s' '%s'", opts.location, opts.dir)
	if opts.recurseSubmodules {
		cloneCmd = fmt.Sprintf("%s --recurse-submodules", cloneCmd)
	}
	if opts.shallowClone {
		// Experiments with shallow clone on AWS hosts suggest that depth 100 is as fast as 1, but 1000 is slower.
		cloneCmd = fmt.Sprintf("%s --depth 100", cloneCmd)
	}
	if opts.branch != "" {
		cloneCmd = fmt.Sprintf("%s --branch '%s'", cloneCmd, opts.branch)
	}

	return []string{
		cloneCmd,
		fmt.Sprintf("cd %s", opts.dir),
	}, nil
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
		return errors.Errorf("error parsing '%s' params: value for directory "+
			"must not be blank", c.Name())
	}

	return nil
}

func (c *gitFetchProject) buildCloneCommand(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, opts cloneOpts) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf("rm -rf %s", c.Directory),
	}

	cloneCmd, err := opts.getCloneCommand()
	if err != nil {
		return nil, errors.Wrap(err, "error getting command to clone repo")
	}
	gitCommands = append(gitCommands, cloneCmd...)

	// if there's a PR checkout the ref containing the changes
	if conf.GithubPatchData.PRNumber != 0 {
		var ref, commitToTest, branchName string
		if conf.Task.Requester == evergreen.MergeTestRequester {
			// proceed if github has confirmed this pr is mergeable. If it hasn't checked, this request
			// will make it check.
			// https://docs.github.com/en/rest/guides/getting-started-with-the-git-database-api#checking-mergeability-of-pull-requests
			commitToTest, err = c.waitForMergeableCheck(ctx, logger, opts, conf.GithubPatchData.PRNumber)
			if err != nil {
				logger.Task().Error(errors.Wrap(err, "error checking if pull request is mergeable"))
				commitToTest = conf.GithubPatchData.HeadHash
				logger.Task().Warning(fmt.Sprintf("because errors were encountered trying to retrieve the pull request, we will use the last recorded hash to test (%s)", commitToTest))
			}
			ref = "merge"
			branchName = fmt.Sprintf("evg-merge-test-%s", utility.RandomString())
		} else {
			// Github creates a ref called refs/pull/[pr number]/head
			// that provides the entire tree of changes, including merges
			ref = "head"
			commitToTest = conf.GithubPatchData.HeadHash
			branchName = fmt.Sprintf("evg-pr-test-%s", utility.RandomString())
		}
		if commitToTest != "" {
			gitCommands = append(gitCommands, []string{
				fmt.Sprintf(`git fetch origin "pull/%d/%s:%s"`, conf.GithubPatchData.PRNumber, ref, branchName),
				fmt.Sprintf(`git checkout "%s"`, branchName),
				fmt.Sprintf("git reset --hard %s", commitToTest),
			}...)
		}

	} else {
		if opts.shallowClone {
			gitCommands = append(gitCommands, fmt.Sprintf("git log HEAD..%s || git fetch --unshallow", conf.Task.Revision))
		}
		if !opts.mergeTestRequester {
			gitCommands = append(gitCommands, fmt.Sprintf("git reset --hard %s", conf.Task.Revision))
		}
	}
	gitCommands = append(gitCommands, "git log --oneline -n 10")

	return gitCommands, nil
}

func (c *gitFetchProject) waitForMergeableCheck(ctx context.Context, logger client.LoggerProducer, opts cloneOpts, prNum int) (string, error) {
	var mergeSHA string
	httpClient := utility.GetOAuth2HTTPClient(opts.token)
	defer utility.PutHTTPClient(httpClient)
	githubClient := github.NewClient(httpClient)
	const (
		getPRAttempts      = 8
		getPRRetryMinDelay = time.Second
		getPRRetryMaxDelay = 15 * time.Second
	)
	err := utility.Retry(ctx, func() (bool, error) {
		pr, _, err := githubClient.PullRequests.Get(ctx, opts.owner, opts.repo, prNum)
		if err != nil {
			return false, errors.Wrap(err, "error getting pull request data from Github")
		}
		if pr.Mergeable == nil {
			logger.Execution().Info("Mergeable check not ready")
			return true, nil
		}
		if *pr.Mergeable {
			if pr.MergeCommitSHA != nil {
				mergeSHA = *pr.MergeCommitSHA
			} else {
				return false, errors.New("Pull request is mergeable but Github has not created a merge branch")
			}
		} else {
			return false, errors.New("Pull request is not mergeable. This likely means a merge conflict was just introduced")
		}
		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: getPRAttempts,
		MinDelay:    getPRRetryMinDelay,
		MaxDelay:    getPRRetryMaxDelay,
	}) // Retry roughly after 1, 2, 4, 8, 15, 15, 15, seconds, or 1 minute.

	return mergeSHA, err
}

func (c *gitFetchProject) buildModuleCloneCommand(conf *internal.TaskConfig, opts cloneOpts, ref string, modulePatch *patch.ModulePatch) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
	}
	if opts.location == "" {
		return nil, errors.New("empty repository URI")
	}
	if opts.dir == "" {
		return nil, errors.New("empty clone path")
	}
	if ref == "" && !isGitHubPRModulePatch(conf, modulePatch) {
		return nil, errors.New("empty ref/branch to check out")
	}

	cloneCmd, err := opts.getCloneCommand()
	if err != nil {
		return nil, errors.Wrap(err, "error getting command to clone repo")
	}
	gitCommands = append(gitCommands, cloneCmd...)

	if isGitHubPRModulePatch(conf, modulePatch) {
		branchName := fmt.Sprintf("evg-merge-test-%s", utility.RandomString())
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
// Retries some number of times before failing
func (c *gitFetchProject) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	const (
		fetchRetryMinDelay = time.Second
		fetchRetryMaxDelay = 10 * time.Second
	)
	err := utility.Retry(
		ctx,
		func() (bool, error) {
			err := c.executeLoop(ctx, comm, logger, conf)
			if err != nil {
				return true, err
			}
			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: GitFetchProjectRetries,
			MinDelay:    fetchRetryMinDelay,
			MaxDelay:    fetchRetryMaxDelay,
		})
	if err != nil {
		logger.Task().Error(message.WrapError(err, message.Fields{
			"operation":    "git.get_project",
			"message":      "cloning failed",
			"num_attempts": GitFetchProjectRetries,
			"owner":        conf.ProjectRef.Owner,
			"repo":         conf.ProjectRef.Repo,
			"branch":       conf.ProjectRef.Branch,
		}))
	}
	return err
}

func (c *gitFetchProject) executeLoop(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	var err error
	// expand the github parameters before running the task
	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "error expanding github parameters")
	}

	var projectMethod string
	var projectToken string
	projectMethod, projectToken, err = getProjectMethodAndToken(c.Token, conf.Expansions.Get(evergreen.GlobalGitHubTokenExpansion), conf.Distro.CloneMethod)
	if err != nil {
		return errors.Wrap(err, "failed to get method of cloning and token")
	}
	opts := cloneOpts{
		method:             projectMethod,
		owner:              conf.ProjectRef.Owner,
		repo:               conf.ProjectRef.Repo,
		branch:             conf.ProjectRef.Branch,
		dir:                c.Directory,
		token:              projectToken,
		shallowClone:       c.ShallowClone && !conf.Distro.DisableShallowClone,
		recurseSubmodules:  c.RecurseSubmodules,
		mergeTestRequester: conf.Task.Requester == evergreen.MergeTestRequester,
	}
	if err = opts.setLocation(); err != nil {
		return errors.Wrap(err, "failed to set location to clone from")
	}
	if err = opts.validate(); err != nil {
		return errors.Wrap(err, "could not validate options for cloning")
	}

	gitCommands, err := c.buildCloneCommand(ctx, conf, logger, opts)
	if err != nil {
		return err
	}

	stdErr := noopWriteCloser{
		&bytes.Buffer{},
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jpm := c.JasperManager()

	fetchScript := strings.Join(gitCommands, "\n")
	fetchSourceCmd := jpm.CreateCommand(ctx).Add([]string{"bash", "-c", fetchScript}).Directory(conf.WorkDir).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorWriter(stdErr)

	logger.Execution().Info("Fetching source from git...")
	redactedCmds := fetchScript
	if opts.token != "" {
		redactedCmds = strings.Replace(redactedCmds, opts.token, "[redacted oauth token]", -1)
	}
	logger.Execution().Debug(fmt.Sprintf("Commands are: %s", redactedCmds))

	err = fetchSourceCmd.Run(ctx)
	errorOutput := stdErr.String()
	if errorOutput != "" {
		if opts.token != "" {
			errorOutput = strings.Replace(errorOutput, opts.token, "[redacted oauth token]", -1)
		}
		logger.Execution().Error(errorOutput)
	}
	if err != nil {
		return errors.Wrap(err, "problem running fetch command")
	}

	var p *patch.Patch
	// additionalPatches is used by evergreen internally and specifies additional patches to
	// apply (for commit queue merges testing with changes from prior tasks). Patches are applied in the
	// order returned, with the main patch being applied last
	var additionalPatches []string
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if evergreen.IsPatchRequester(conf.Task.Requester) {
		logger.Execution().Info("Fetching patch.")
		p, err = comm.GetTaskPatch(ctx, td, "")
		if err != nil {
			return errors.Wrap(err, "Failed to get patch")
		}

		if conf.Task.Requester == evergreen.MergeTestRequester {
			additionalPatches, err = comm.GetAdditionalPatches(ctx, conf.Task.Version, td)
			if err != nil {
				return errors.Wrap(err, "Failed to get additional patches")
			}
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

		moduleBase := filepath.ToSlash(filepath.Join(expandModulePrefix(conf, module.Name, module.Prefix, logger), module.Name))

		var revision string
		// use submodule revisions based on the main patch. If there is a need in the future,
		// this could maybe use the most recent submodule revision of all requested patches.
		// We ignore set-module changes for commit queue, since we should verify HEAD before merging.
		if p != nil {
			patchModule := p.FindModule(moduleName)
			if patchModule != nil {
				if conf.Task.Requester == evergreen.MergeTestRequester {
					revision = module.Branch
					c.logModuleRevision(logger, revision, moduleName, "defaulting to HEAD for merge")
				} else {
					revision = patchModule.Githash
					if revision != "" {
						c.logModuleRevision(logger, revision, moduleName, "specified in set-module")
					}
				}
			}
		}
		if revision == "" {
			revision = c.Revisions[moduleName]
			if revision != "" {
				c.logModuleRevision(logger, revision, moduleName, "specified as parameter to git.get_project")
			}
		}
		if revision == "" {
			revision = conf.Expansions.Get(moduleRevExpansionName(moduleName))
			if revision != "" {
				c.logModuleRevision(logger, revision, moduleName, "from manifest")
			}
		}
		// if there is no revision, then use the revision from the module, then branch name
		if revision == "" {
			if module.Ref != "" {
				revision = module.Ref
				c.logModuleRevision(logger, revision, moduleName, "ref field in config file")
			} else {
				revision = module.Branch
				c.logModuleRevision(logger, revision, moduleName, "branch field in config file")
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

		opts := cloneOpts{
			location: module.Repo,
			owner:    owner,
			repo:     repo,
			branch:   "",
			dir:      moduleBase,
		}
		// Module's location takes precedence over the project-level clone
		// method.
		if strings.Contains(opts.location, "git@github.com:") {
			opts.method = distro.CloneMethodLegacySSH
		} else {
			opts.method = projectMethod
			opts.token = projectToken
		}
		if err = opts.validate(); err != nil {
			return errors.Wrap(err, "could not validate options for cloning")
		}

		var moduleCmds []string
		moduleCmds, err = c.buildModuleCloneCommand(conf, opts, revision, modulePatch)
		if err != nil {
			return err
		}

		err = jpm.CreateCommand(ctx).Add([]string{"bash", "-c", strings.Join(moduleCmds, "\n")}).
			Directory(filepath.ToSlash(filepath.Join(conf.WorkDir, c.Directory))).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorWriter(stdErr).Run(ctx)

		errOutput := stdErr.String()
		if errOutput != "" {
			if opts.token != "" {
				errOutput = strings.Replace(errOutput, opts.token, "[redacted oauth token]", -1)
			}
			logger.Execution().Info(errOutput)
		}
		if err != nil {
			return errors.Wrap(err, "problem with git command")
		}
	}

	if conf.Task.Requester == evergreen.MergeTestRequester && !conf.Task.CommitQueueMerge {
		for _, patchId := range additionalPatches {
			logger.Task().Infof("applying changes from previous commit queue patch '%s'", patchId)
			newPatch, err := comm.GetTaskPatch(ctx, td, patchId)
			if err != nil {
				return errors.Wrap(err, "unable to get additional patch")
			}
			if newPatch == nil {
				return errors.New("additional patch not found")
			}
			if err = c.getPatchContents(ctx, comm, logger, conf, newPatch); err != nil {
				return errors.Wrap(err, "Failed to get patch contents")
			}
			if err = c.applyPatch(ctx, logger, conf, reorderPatches(newPatch.Patches)); err != nil {
				return errors.Wrapf(err, "error applying patch '%s'", newPatch.Id.Hex())
			}
			logger.Task().Infof("applied changes from previous commit queue patch '%s'", patchId)
		}
	}

	//Apply patches if this is a patch and we haven't already gotten the changes from a PR
	if evergreen.IsPatchRequester(conf.Task.Requester) && conf.GithubPatchData.PRNumber == 0 {
		if err = c.getPatchContents(ctx, comm, logger, conf, p); err != nil {
			err = errors.Wrap(err, "Failed to get patch contents")
			logger.Execution().Error(err.Error())
			return err
		}

		// in order for the main commit's manifest to include module changes commit queue
		// commits need to be in the correct order, first modules and then the main patch
		// reorder patches so the main patch gets applied last
		if err = c.applyPatch(ctx, logger, conf, reorderPatches(p.Patches)); err != nil {
			err = errors.Wrap(err, "Failed to apply patch")
			logger.Execution().Error(err.Error())
			return err
		}
	}

	return nil
}

// reorder a slice of ModulePatches so the main patch is last
func reorderPatches(originalPatches []patch.ModulePatch) []patch.ModulePatch {
	patches := make([]patch.ModulePatch, len(originalPatches))
	index := 0
	for _, mp := range originalPatches {
		if mp.ModuleName == "" {
			patches[len(patches)-1] = mp
		} else {
			patches[index] = mp
			index++
		}
	}

	return patches
}

func (c *gitFetchProject) logModuleRevision(logger client.LoggerProducer, revision, module, reason string) {
	logger.Execution().Infof("Using revision/ref '%s' for module '%s' (reason: %s)", revision, module, reason)
}

// getPatchContents() dereferences any patch files that are stored externally, fetching them from
// the API server, and setting them into the patch object.
func (c *gitFetchProject) getPatchContents(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, conf *internal.TaskConfig, patch *patch.Patch) error {

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

// getApplyCommand determines the patch type. If the patch is a mailbox-style
// patch, it uses git-am (see https://git-scm.com/docs/git-am), otherwise
// it uses git apply
func (c *gitFetchProject) getApplyCommand(patchFile string) (string, error) {
	isMBP, err := patch.IsMailbox(patchFile)
	if err != nil {
		return "", errors.Wrap(err, "can't check patch type")
	}

	if isMBP {
		committerName := defaultCommitterName
		committerEmail := defaultCommitterEmail
		if len(c.CommitterName) > 0 {
			committerName = c.CommitterName
		}
		if len(c.CommitterEmail) > 0 {
			committerEmail = c.CommitterEmail
		}
		return fmt.Sprintf(`GIT_COMMITTER_NAME="%s" GIT_COMMITTER_EMAIL="%s" git am --keep-cr --keep < "%s"`, committerName, committerEmail, patchFile), nil
	}

	return fmt.Sprintf("git apply --binary --index < '%s'", patchFile), nil
}

// getPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed, except for apply. If the patch is empty it will not apply the patch.
func getPatchCommands(modulePatch patch.ModulePatch, conf *internal.TaskConfig, dir, patchPath string) []string {
	patchCommands := []string{
		fmt.Sprintf("set -o xtrace"),
		fmt.Sprintf("set -o errexit"),
		fmt.Sprintf("ls"),
		fmt.Sprintf("cd '%s'", dir),
	}
	if conf.Task.Requester != evergreen.MergeTestRequester {
		patchCommands = append(patchCommands, fmt.Sprintf("git reset --hard '%s'", modulePatch.Githash))
	}

	if modulePatch.PatchSet.Patch == "" {
		return patchCommands
	}
	return append(patchCommands, fmt.Sprintf("git apply --stat '%v' || true", patchPath))
}

// applyPatch is used by the agent to copy patch data onto disk
// and then call the necessary git commands to apply the patch file
func (c *gitFetchProject) applyPatch(ctx context.Context, logger client.LoggerProducer,
	conf *internal.TaskConfig, patches []patch.ModulePatch) error {

	jpm := c.JasperManager()

	// patch sets and contain multiple patches, some of them for modules
	for _, patchPart := range patches {
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
			if !utility.StringSliceContains(conf.BuildVariant.Modules, module.Name) {
				logger.Execution().Infof(
					"Skipping patch for module %v: the current build variant does not use it",
					module.Name)
				continue
			}

			dir = filepath.Join(c.Directory, expandModulePrefix(conf, module.Name, module.Prefix, logger), module.Name)
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
		defer func() { //nolint: evg-lint
			grip.Error(tempFile.Close())
			grip.Error(os.Remove(tempFile.Name()))
		}()
		_, err = io.WriteString(tempFile, patchPart.PatchSet.Patch)
		if err != nil {
			return errors.WithStack(err)
		}
		tempAbsPath := tempFile.Name()

		// this applies the patch using the patch files in the temp directory
		patchCommandStrings := getPatchCommands(patchPart, conf, dir, tempAbsPath)
		applyCommand, err := c.getApplyCommand(tempAbsPath)
		if err != nil {
			logger.Execution().Error("Could not to determine patch type")
			return errors.WithStack(err)
		}
		patchCommandStrings = append(patchCommandStrings, applyCommand)
		cmdsJoined := strings.Join(patchCommandStrings, "\n")

		cmd := jpm.CreateCommand(ctx).Directory(conf.WorkDir).Add([]string{"bash", "-c", cmdsJoined}).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())

		if err = cmd.Run(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func isGitHubPRModulePatch(conf *internal.TaskConfig, modulePatch *patch.ModulePatch) bool {
	isGitHubMergeTest := conf.GithubPatchData.PRNumber != 0
	patchProvided := (modulePatch != nil) && (modulePatch.PatchSet.Patch != "")

	return isGitHubMergeTest && patchProvided
}

type noopWriteCloser struct {
	*bytes.Buffer
}

func (noopWriteCloser) Close() error { return nil }
