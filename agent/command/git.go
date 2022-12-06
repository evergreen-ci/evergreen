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
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

const (
	GitFetchProjectRetries = 5
	defaultCommitterName   = "Evergreen Agent"
	defaultCommitterEmail  = "no-reply@evergreen.mongodb.com"
	shallowCloneDepth      = 100
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

	// ShallowClone sets CloneDepth to 100, and is kept for backwards compatibility.
	ShallowClone bool `mapstructure:"shallow_clone"`
	CloneDepth   int  `mapstructure:"clone_depth"`

	RecurseSubmodules bool `mapstructure:"recurse_submodules"`

	CommitterName string `mapstructure:"committer_name"`

	CommitterEmail string `mapstructure:"committer_email"`

	CloneParams string `mapstructure:"clone_params"`

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
	cloneParams        string
	recurseSubmodules  bool
	mergeTestRequester bool
	cloneDepth         int
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
		catcher.Wrapf(evergreen.ValidateCloneMethod(opts.method), "invalid clone method '%s'", opts.method)
	}
	if opts.method == evergreen.CloneMethodOAuth && opts.token == "" {
		catcher.New("cannot clone using OAuth if token is not set")
	}
	if opts.cloneDepth < 0 {
		catcher.New("clone depth cannot be negative")
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
	case "", evergreen.CloneMethodLegacySSH:
		opts.location = opts.sshLocation()
	case evergreen.CloneMethodOAuth:
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
		return evergreen.CloneMethodOAuth, token, err
	}
	token, err := parseToken(globalToken)
	if err != nil {
		return evergreen.CloneMethodLegacySSH, "", err
	}

	switch globalCloneMethod {
	// No clone method specified is equivalent to using legacy SSH.
	case "", evergreen.CloneMethodLegacySSH:
		return evergreen.CloneMethodLegacySSH, token, nil
	case evergreen.CloneMethodOAuth:
		if token == "" {
			return evergreen.CloneMethodLegacySSH, "", errors.New("cannot clone using OAuth if explicit token from parameter and global token are both empty")
		}
		token, err := parseToken(globalToken)
		return evergreen.CloneMethodOAuth, token, err
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
		return nil, errors.Wrap(err, "invalid clone command options")
	}
	switch opts.method {
	case "", evergreen.CloneMethodLegacySSH:
		return opts.buildSSHCloneCommand()
	case evergreen.CloneMethodOAuth:
		return opts.buildHTTPCloneCommand()
	}
	return nil, errors.New("unrecognized clone method in options")
}

func (opts cloneOpts) buildHTTPCloneCommand() ([]string, error) {
	urlLocation, err := url.Parse(opts.location)
	if err != nil {
		return nil, errors.Wrap(err, "parsing URL from location")
	}
	clone := fmt.Sprintf("git clone %s '%s'", thirdparty.FormGitUrl(urlLocation.Host, opts.owner, opts.repo, opts.token), opts.dir)
	if opts.recurseSubmodules {
		clone = fmt.Sprintf("%s --recurse-submodules", clone)
	}
	if opts.cloneDepth > 0 {
		clone = fmt.Sprintf("%s --depth %d", clone, opts.cloneDepth)
	}
	if opts.branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, opts.branch)
	}
	if opts.cloneParams != "" {
		clone = fmt.Sprintf("%s %s", clone, opts.cloneParams)
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
	if opts.cloneDepth > 0 {
		cloneCmd = fmt.Sprintf("%s --depth %d", cloneCmd, opts.cloneDepth)
	}
	if opts.branch != "" {
		cloneCmd = fmt.Sprintf("%s --branch '%s'", cloneCmd, opts.branch)
	}
	if opts.cloneParams != "" {
		cloneCmd = fmt.Sprintf("%s %s", cloneCmd, opts.cloneParams)
	}

	return []string{
		cloneCmd,
		fmt.Sprintf("cd %s", opts.dir),
	}, nil
}

func moduleRevExpansionName(name string) string { return fmt.Sprintf("%s_rev", name) }

// Load performs a GET on /manifest/load
func (c *gitFetchProject) manifestLoad(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	manifest, err := comm.GetManifest(ctx, td)
	if err != nil {
		return errors.Wrapf(err, "loading manifest for task '%s'", td.ID)
	}

	for moduleName := range manifest.Modules {
		// put the url for the module in the expansions
		conf.Expansions.Put(moduleRevExpansionName(moduleName), manifest.Modules[moduleName].Revision)
		conf.Expansions.Put(fmt.Sprintf("%s_branch", moduleName), manifest.Modules[moduleName].Branch)
		conf.Expansions.Put(fmt.Sprintf("%s_repo", moduleName), manifest.Modules[moduleName].Repo)
		conf.Expansions.Put(fmt.Sprintf("%s_owner", moduleName), manifest.Modules[moduleName].Owner)
	}

	logger.Execution().Info("Manifest loaded successfully.")
	return nil
}

func gitFetchProjectFactory() Command   { return &gitFetchProject{} }
func (c *gitFetchProject) Name() string { return "git.get_project" }

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (c *gitFetchProject) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Directory == "" {
		return errors.New("directory must not be blank")
	}

	return nil
}

func (c *gitFetchProject) buildCloneCommand(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, opts cloneOpts) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf("rm -rf %s", c.Directory),
	}

	cloneCmd, err := opts.getCloneCommand()
	if err != nil {
		return nil, errors.Wrap(err, "getting command to clone repo")
	}
	gitCommands = append(gitCommands, cloneCmd...)

	// if there's a PR checkout the ref containing the changes
	if isGitHub(conf) {
		var ref, branchName, commitToTest string
		if conf.Task.Requester == evergreen.MergeTestRequester {
			// Proceed if github has confirmed this pr is mergeable. If it hasn't checked, this request
			// will make it check.
			commitToTest, err = c.waitForMergeableCheck(ctx, comm, logger, conf, opts)
			if err != nil {
				commitToTest = conf.GithubPatchData.HeadHash
				logger.Task().Errorf("Error checking if pull request is mergeable: %s", err)
				logger.Task().Warningf("Because errors were encountered trying to retrieve the pull request, we will use the last recorded hash to test (%s).", commitToTest)
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
		if opts.cloneDepth > 0 {
			// If this git log fails, then we know the clone is too shallow so we unshallow before reset.
			gitCommands = append(gitCommands, fmt.Sprintf("git log HEAD..%s || git fetch --unshallow", conf.Task.Revision))
		}
		if !opts.mergeTestRequester {
			gitCommands = append(gitCommands, fmt.Sprintf("git reset --hard %s", conf.Task.Revision))
		}
	}
	gitCommands = append(gitCommands, "git log --oneline -n 10")

	return gitCommands, nil
}

func (c *gitFetchProject) waitForMergeableCheck(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, opts cloneOpts) (string, error) {
	var mergeSHA string

	const (
		getPRAttempts      = 3
		getPRRetryMinDelay = time.Second
		getPRRetryMaxDelay = 15 * time.Second
	)
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	err := utility.Retry(ctx, func() (bool, error) {
		info, err := comm.GetPullRequestInfo(ctx, td, conf.GithubPatchData.PRNumber, opts.owner, opts.repo)
		if err != nil {
			return false, errors.Wrap(err, "getting pull request data from GitHub")
		}
		if info.Mergeable == nil {
			logger.Execution().Info("Mergeable check is not ready.")
			return true, nil
		}
		if *info.Mergeable {
			if info.MergeCommitSHA != "" {
				mergeSHA = info.MergeCommitSHA
			} else {
				return false, errors.New("pull request is mergeable but GitHub has not created a merge branch")
			}
		} else {
			return false, errors.New("pull request is not mergeable, which likely means a merge conflict was just introduced")
		}
		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: getPRAttempts,
		MinDelay:    getPRRetryMinDelay,
		MaxDelay:    getPRRetryMaxDelay,
	})

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
		return nil, errors.Wrap(err, "getting command to clone repo")
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

func (c *gitFetchProject) opts(projectMethod, projectToken string, logger client.LoggerProducer, conf *internal.TaskConfig) (cloneOpts, error) {
	shallowCloneEnabled := conf.Distro == nil || !conf.Distro.DisableShallowClone
	opts := cloneOpts{
		method:             projectMethod,
		owner:              conf.ProjectRef.Owner,
		repo:               conf.ProjectRef.Repo,
		branch:             conf.ProjectRef.Branch,
		dir:                c.Directory,
		token:              projectToken,
		cloneParams:        c.CloneParams,
		recurseSubmodules:  c.RecurseSubmodules,
		mergeTestRequester: conf.Task.Requester == evergreen.MergeTestRequester,
	}
	cloneDepth := c.CloneDepth
	if cloneDepth == 0 && c.ShallowClone {
		// Experiments with shallow clone on AWS hosts suggest that depth 100 is as fast as 1, but 1000 is slower.
		cloneDepth = shallowCloneDepth
	}
	if !shallowCloneEnabled && cloneDepth != 0 {
		logger.Task().Infof("Clone depth is disabled for this distro; ignoring-user specified clone depth.")
	} else {
		opts.cloneDepth = cloneDepth
		if c.CloneDepth != 0 && c.ShallowClone {
			logger.Task().Infof("Specified clone depth of %d will be used instead of shallow_clone (which uses depth %d).", opts.cloneDepth, shallowCloneDepth)
		}
	}

	if err := opts.setLocation(); err != nil {
		return opts, errors.Wrap(err, "setting location to clone from")
	}
	if err := opts.validate(); err != nil {
		return opts, errors.Wrap(err, "validating clone options")
	}
	return opts, nil
}

// Execute gets the source code required by the project
// Retries some number of times before failing
func (c *gitFetchProject) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	const (
		fetchRetryMinDelay = time.Second
		fetchRetryMaxDelay = 10 * time.Second
	)

	err := c.manifestLoad(ctx, comm, logger, conf)
	if err != nil {
		return errors.Wrap(err, "loading manifest")
	}

	if err = util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	projectMethod, projectToken, err := getProjectMethodAndToken(c.Token, conf.Expansions.Get(evergreen.GlobalGitHubTokenExpansion), conf.GetCloneMethod())
	if err != nil {
		return errors.Wrap(err, "getting method of cloning and token")
	}

	var opts cloneOpts
	opts, err = c.opts(projectMethod, projectToken, logger, conf)
	if err != nil {
		return err
	}

	err = utility.Retry(
		ctx,
		func() (bool, error) {
			err := c.fetch(ctx, comm, logger, conf, opts)
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

func (c *gitFetchProject) fetchSource(ctx context.Context,
	comm client.Communicator,
	logger client.LoggerProducer,
	conf *internal.TaskConfig,
	jpm jasper.Manager,
	opts cloneOpts) error {

	gitCommands, err := c.buildCloneCommand(ctx, comm, logger, conf, opts)
	if err != nil {
		return err
	}
	fetchScript := strings.Join(gitCommands, "\n")

	// This needs to use a thread-safe buffer just in case the context errors
	// (e.g. due to a timeout) while the command is running. A non-thread-safe
	// buffer is only safe to read once the command exits, guaranteeing that all
	// output is finished writing. However, if the context errors, Run will
	// return early and will stop waiting for the command to exit. In the
	// context error case, this thread and the still-running command may race to
	// read/write the buffer, so the buffer has to be thread-safe.
	stdErr := utility.MakeSafeBuffer(bytes.Buffer{})
	fetchSourceCmd := jpm.CreateCommand(ctx).Add([]string{"bash", "-c", fetchScript}).Directory(conf.WorkDir).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorWriter(stdErr)

	logger.Execution().Info("Fetching source from git...")
	redactedCmds := fetchScript
	if opts.token != "" {
		redactedCmds = strings.Replace(redactedCmds, opts.token, "[redacted oauth token]", -1)
	}
	logger.Execution().Debugf("Commands are: %s", redactedCmds)

	err = fetchSourceCmd.Run(ctx)
	out := stdErr.String()
	if out != "" {
		if opts.token != "" {
			out = strings.Replace(out, opts.token, "[redacted oauth token]", -1)
		}
		logger.Execution().Error(out)
	}
	return err
}

func (c *gitFetchProject) fetchAdditionalPatches(ctx context.Context,
	comm client.Communicator,
	logger client.LoggerProducer,
	conf *internal.TaskConfig,
	td client.TaskData) ([]string, error) {

	logger.Execution().Info("Fetching additional patches.")
	additionalPatches, err := comm.GetAdditionalPatches(ctx, conf.Task.Version, td)
	if err != nil {
		return nil, errors.Wrap(err, "getting additional patches")
	}
	return additionalPatches, nil
}

func (c *gitFetchProject) fetchModuleSource(ctx context.Context,
	conf *internal.TaskConfig,
	logger client.LoggerProducer,
	jpm jasper.Manager,
	projectMethod string,
	projectToken string,
	p *patch.Patch,
	moduleName string) error {

	var err error
	logger.Execution().Infof("Fetching module '%s'.", moduleName)

	var module *model.Module
	module, err = conf.Project.GetModuleByName(moduleName)
	if err != nil {
		return errors.Wrapf(err, "getting module '%s'", moduleName)
	}
	if module == nil {
		return errors.Errorf("module '%s' not found", moduleName)
	}

	moduleBase := filepath.ToSlash(filepath.Join(expandModulePrefix(conf, module.Name, module.Prefix, logger), module.Name))

	// use submodule revisions based on the main patch. If there is a need in the future,
	// this could maybe use the most recent submodule revision of all requested patches.
	// We ignore set-module changes for commit queue, since we should verify HEAD before merging.
	var modulePatch *patch.ModulePatch
	var revision string
	if p != nil {
		modulePatch := p.FindModule(moduleName)
		if modulePatch != nil {
			if conf.Task.Requester == evergreen.MergeTestRequester {
				revision = module.Branch
				c.logModuleRevision(logger, revision, moduleName, "defaulting to HEAD for merge")
			} else {
				revision = modulePatch.Githash
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
		return err
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
		opts.method = evergreen.CloneMethodLegacySSH
	} else {
		opts.method = projectMethod
		opts.token = projectToken
	}
	if err = opts.validate(); err != nil {
		return errors.Wrap(err, "validating clone options")
	}

	var moduleCmds []string
	moduleCmds, err = c.buildModuleCloneCommand(conf, opts, revision, modulePatch)
	if err != nil {
		return err
	}

	// This needs to use a thread-safe buffer just in case the context errors
	// (e.g. due to a timeout) while the command is running. A non-thread-safe
	// buffer is only safe to read once the command exits, guaranteeing that all
	// output is finished writing. However, if the context errors, Run will
	// return early and will stop waiting for the command to exit. In the
	// context error case, this thread and the still-running command may race to
	// read/write the buffer, so the buffer has to be thread-safe.
	stdErr := utility.MakeSafeBuffer(bytes.Buffer{})
	err = jpm.CreateCommand(ctx).Add([]string{"bash", "-c", strings.Join(moduleCmds, "\n")}).
		Directory(filepath.ToSlash(getJoinedWithWorkDir(conf, c.Directory))).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorWriter(stdErr).Run(ctx)

	errOutput := stdErr.String()
	if errOutput != "" {
		if opts.token != "" {
			errOutput = strings.Replace(errOutput, opts.token, "[redacted oauth token]", -1)
		}
		logger.Execution().Info(errOutput)
	}
	return err
}

func (c *gitFetchProject) applyAdditionalPatch(ctx context.Context,
	comm client.Communicator,
	logger client.LoggerProducer,
	conf *internal.TaskConfig,
	td client.TaskData,
	patchId string) error {
	logger.Task().Infof("Applying changes from previous commit queue patch '%s'.", patchId)
	newPatch, err := comm.GetTaskPatch(ctx, td, patchId)
	if err != nil {
		return errors.Wrap(err, "getting additional patch")
	}
	if newPatch == nil {
		return errors.New("additional patch not found")
	}
	if err = c.getPatchContents(ctx, comm, logger, conf, newPatch); err != nil {
		return errors.Wrap(err, "getting patch contents")
	}
	if err = c.applyPatch(ctx, logger, conf, reorderPatches(newPatch.Patches)); err != nil {
		logger.Task().Warning("Failed to apply previous commit queue patch; try rebasing onto HEAD.")
		return errors.Wrapf(err, "applying patch '%s'", newPatch.Id.Hex())
	}
	logger.Task().Infof("Applied changes from previous commit queue patch '%s'", patchId)
	return nil
}

func (c *gitFetchProject) fetch(ctx context.Context,
	comm client.Communicator,
	logger client.LoggerProducer,
	conf *internal.TaskConfig,
	opts cloneOpts) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jpm := c.JasperManager()

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	// Additional patches are for commit queue batch execution. Patches
	// will be applied in the order returned, with the main patch being
	// applied last. We check this first to avoid cloning if
	// the task isn't on the commit queue anymore.
	var err error
	var additionalPatches []string
	if conf.Task.Requester == evergreen.MergeTestRequester {
		additionalPatches, err = c.fetchAdditionalPatches(ctx, comm, logger, conf, td)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Clone the project.
	if err = c.fetchSource(ctx, comm, logger, conf, jpm, opts); err != nil {
		return errors.Wrap(err, "problem running fetch command")
	}

	// Retrieve the patch for the version if one exists.
	var p *patch.Patch
	if evergreen.IsPatchRequester(conf.Task.Requester) {
		logger.Execution().Info("Fetching patch.")
		p, err = comm.GetTaskPatch(ctx, td, "")
		if err != nil {
			return errors.Wrap(err, "getting patch for task")
		}
	}

	// Clone the project's modules.
	for _, moduleName := range conf.BuildVariant.Modules {
		if err := ctx.Err(); err != nil {
			return errors.Wrapf(err, "canceled while applying module '%s'", moduleName)
		}
		err = c.fetchModuleSource(ctx, conf, logger, jpm, opts.method, opts.token, p, moduleName)
		if err != nil {
			logger.Execution().Error(errors.Wrap(err, "fetching module source"))
		}
	}

	// Apply additional patches for commit queue batch execution.
	if conf.Task.Requester == evergreen.MergeTestRequester && !conf.Task.CommitQueueMerge {
		for _, patchId := range additionalPatches {
			err := c.applyAdditionalPatch(ctx, comm, logger, conf, td, patchId)
			if err != nil {
				return err
			}
		}
	}

	// Apply patches if this is a patch and we haven't already gotten the changes from a PR
	if evergreen.IsPatchRequester(conf.Task.Requester) && !isGitHub(conf) {
		if err = c.getPatchContents(ctx, comm, logger, conf, p); err != nil {
			err = errors.Wrap(err, "getting patch contents")
			logger.Execution().Error(err.Error())
			return err
		}

		// in order for the main commit's manifest to include module changes commit queue
		// commits need to be in the correct order, first modules and then the main patch
		// reorder patches so the main patch gets applied last
		if err = c.applyPatch(ctx, logger, conf, reorderPatches(p.Patches)); err != nil {
			err = errors.Wrap(err, "applying patch")
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
	logger.Execution().Infof("Using revision/ref '%s' for module '%s' (reason: %s).", revision, module, reason)
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

		if err := ctx.Err(); err != nil {
			return errors.Wrapf(err, "canceled while getting patch file '%s'", patchPart.ModuleName)
		}

		// otherwise, fetch the contents and load it into the patch object
		logger.Execution().Infof("Fetching patch contents for patch file '%s'.", patchPart.PatchSet.PatchFileId)

		result, err := comm.GetPatchFile(ctx, td, patchPart.PatchSet.PatchFileId)
		if err != nil {
			return errors.Wrapf(err, "getting patch file")
		}

		patch.Patches[i].PatchSet.Patch = result
	}
	return nil
}

// getApplyCommand determines the patch type. If the patch is a mailbox-style
// patch, it uses git-am (see https://git-scm.com/docs/git-am), otherwise
// it uses git apply
func (c *gitFetchProject) getApplyCommand(patchFile string, conf *internal.TaskConfig) (string, error) {
	useGitAm, err := isMailboxPatch(patchFile, conf)
	if err != nil {
		return "", err
	}

	if useGitAm {
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

func isMailboxPatch(patchFile string, conf *internal.TaskConfig) (bool, error) {
	isMBP, err := patch.IsMailbox(patchFile)
	if err != nil {
		return false, errors.Wrap(err, "checking patch type")
	}
	return isMBP && conf.Task.DisplayName == evergreen.MergeTaskName, nil
}

// getPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed, except for apply. If the patch is empty it will not apply the patch.
func getPatchCommands(modulePatch patch.ModulePatch, conf *internal.TaskConfig, moduleDir, patchPath string) []string {
	patchCommands := []string{
		"set -o xtrace",
		"set -o errexit",
	}
	if moduleDir != "" {
		patchCommands = append(patchCommands, fmt.Sprintf("cd '%s'", moduleDir))
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
		if err := ctx.Err(); err != nil {
			return errors.Wrapf(err, "canceled while applying module patch '%s'", patchPart.ModuleName)
		}

		var moduleDir string
		if patchPart.ModuleName != "" {
			// if patch is part of a module, apply patch in module root
			module, err := conf.Project.GetModuleByName(patchPart.ModuleName)
			if err != nil {
				return errors.Wrap(err, "getting module")
			}
			if module == nil {
				return errors.Errorf("module '%s' not found", patchPart.ModuleName)
			}

			// skip the module if this build variant does not use it
			if !utility.StringSliceContains(conf.BuildVariant.Modules, module.Name) {
				logger.Execution().Infof(
					"Skipping patch for module '%s': the current build variant does not use it.",
					module.Name)
				continue
			}

			moduleDir = filepath.ToSlash(filepath.Join(expandModulePrefix(conf, module.Name, module.Prefix, logger), module.Name))
		}

		if len(patchPart.PatchSet.Patch) == 0 {
			logger.Execution().Info("Skipping empty patch file...")
			continue

		} else if patchPart.ModuleName == "" {
			logger.Execution().Info("Applying patch with git...")

		} else {
			logger.Execution().Infof("Applying '%s' module patch with git...", patchPart.ModuleName)
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
		patchCommandStrings := getPatchCommands(patchPart, conf, moduleDir, tempAbsPath)
		applyCommand, err := c.getApplyCommand(tempAbsPath, conf)
		if err != nil {
			return errors.Wrap(err, "getting git apply command")
		}
		patchCommandStrings = append(patchCommandStrings, applyCommand)
		cmdsJoined := strings.Join(patchCommandStrings, "\n")

		cmd := jpm.CreateCommand(ctx).Add([]string{"bash", "-c", cmdsJoined}).
			Directory(filepath.ToSlash(getJoinedWithWorkDir(conf, c.Directory))).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Task().GetSender())

		if err = cmd.Run(ctx); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func isGitHubPRModulePatch(conf *internal.TaskConfig, modulePatch *patch.ModulePatch) bool {
	patchProvided := (modulePatch != nil) && (modulePatch.PatchSet.Patch != "")
	return isGitHub(conf) && patchProvided
}

func isGitHub(conf *internal.TaskConfig) bool {
	return conf.GithubPatchData.PRNumber != 0
}

type noopWriteCloser struct {
	*bytes.Buffer
}

func (noopWriteCloser) Close() error { return nil }
