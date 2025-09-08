package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	gitFetchProjectRetries = 5
	shallowCloneDepth      = 100

	gitGetProjectAttribute = "evergreen.command.git_get_project"

	generatedTokenKey = "EVERGREEN_GENERATED_GITHUB_TOKEN"

	// githubMergeQueueInvalidRefError is the error message returned by Git when it fails to find
	// a GitHub merge queue reference.
	githubMergeQueueInvalidRefError = "couldn't find remote ref gh-readonly-queue"
)

var (
	cloneOwnerAttribute   = fmt.Sprintf("%s.clone_owner", gitGetProjectAttribute)
	cloneRepoAttribute    = fmt.Sprintf("%s.clone_repo", gitGetProjectAttribute)
	cloneBranchAttribute  = fmt.Sprintf("%s.clone_branch", gitGetProjectAttribute)
	cloneModuleAttribute  = fmt.Sprintf("%s.clone_module", gitGetProjectAttribute)
	cloneAttemptAttribute = fmt.Sprintf("%s.attempt", gitGetProjectAttribute)
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

	base
}

type cloneOpts struct {
	owner             string
	repo              string
	branch            string
	dir               string
	token             string
	recurseSubmodules bool
	useVerbose        bool
	cloneDepth        int
}

func (opts cloneOpts) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.owner == "", "missing required owner")
	catcher.NewWhen(opts.repo == "", "missing required repo")

	catcher.NewWhen(opts.cloneDepth < 0, "clone depth cannot be negative")
	return catcher.Resolve()
}

// getProjectMethodAndToken returns the project's clone method and token. If
// set, the project token takes precedence over GitHub App token which takes precedence over over global settings.
func getProjectMethodAndToken(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, providedToken string) (string, error) {
	if providedToken != "" {
		token, err := parseToken(providedToken)
		return token, err
	}

	owner := conf.ProjectRef.Owner
	repo := conf.ProjectRef.Repo
	appToken, err := comm.CreateInstallationTokenForClone(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, owner, repo)
	if err != nil {
		return "", errors.Wrap(err, "creating app token")
	}
	if appToken != "" {
		// Redact the token from the logs.
		conf.NewExpansions.Redact(generatedTokenKey, appToken)
	}

	return appToken, nil
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

	gitURL := thirdparty.FormGitURLForApp(opts.owner, opts.repo, opts.token)

	clone := fmt.Sprintf("git clone %s '%s'", gitURL, opts.dir)

	if opts.recurseSubmodules {
		clone = fmt.Sprintf("%s --recurse-submodules", clone)
	}
	if opts.useVerbose {
		clone = fmt.Sprintf("GIT_TRACE=1 %s", clone)
	}
	if opts.cloneDepth > 0 {
		clone = fmt.Sprintf("%s --depth %d", clone, opts.cloneDepth)
	}
	if opts.branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, opts.branch)
	}

	return []string{
		"set +o xtrace",
		fmt.Sprintf(`echo %s`, strconv.Quote(clone)),
		clone,
		"set -o xtrace",
		fmt.Sprintf("cd %s", opts.dir),
	}, nil
}

func moduleRevExpansionName(name string) string { return fmt.Sprintf("%s_rev", name) }

func loadModulesManifestInToExpansions(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig) error {
	manifest, err := comm.GetManifest(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
	if err != nil {
		return errors.Wrapf(err, "loading manifest for task '%s'", conf.Task.Id)
	}

	for moduleName := range manifest.Modules {
		// put the url for the module in the expansions
		conf.NewExpansions.Put(moduleRevExpansionName(moduleName), manifest.Modules[moduleName].Revision)
		conf.NewExpansions.Put(fmt.Sprintf("%s_branch", moduleName), manifest.Modules[moduleName].Branch)
		conf.NewExpansions.Put(fmt.Sprintf("%s_repo", moduleName), manifest.Modules[moduleName].Repo)
		conf.NewExpansions.Put(fmt.Sprintf("%s_owner", moduleName), manifest.Modules[moduleName].Owner)
	}

	return nil
}

func gitFetchProjectFactory() Command   { return &gitFetchProject{} }
func (c *gitFetchProject) Name() string { return "git.get_project" }

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (c *gitFetchProject) ParseParams(params map[string]any) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Directory == "" {
		return errors.New("directory must not be blank")
	}

	return nil
}

func (c *gitFetchProject) buildSourceCloneCommand(conf *internal.TaskConfig, opts cloneOpts) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		fmt.Sprintf("chmod -R 755 %s", c.Directory),
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
		var suffix, localBranchName, remoteBranchName, commitToTest string
		if conf.Task.Requester == evergreen.GithubPRRequester {
			// Github creates a ref called refs/pull/[pr number]/head
			// that provides the entire tree of changes, including merges
			suffix = "/head"
			commitToTest = conf.GithubPatchData.HeadHash
			localBranchName = fmt.Sprintf("evg-pr-test-%s", utility.RandomString())
			remoteBranchName = fmt.Sprintf("pull/%d", conf.GithubPatchData.PRNumber)
		} else if conf.Task.Requester == evergreen.GithubMergeRequester {
			suffix = "" // redundant, included for clarity
			commitToTest = conf.GithubMergeData.HeadSHA
			localBranchName = fmt.Sprintf("evg-mg-test-%s", utility.RandomString())
			// HeadRef looks like "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
			remoteBranchName = conf.GithubMergeData.HeadBranch
		}
		if commitToTest != "" {
			gitCommands = append(gitCommands, []string{
				fmt.Sprintf(`git fetch origin "%s%s:%s"`, remoteBranchName, suffix, localBranchName),
				fmt.Sprintf(`git checkout "%s"`, localBranchName),
				fmt.Sprintf("git reset --hard %s", commitToTest),
			}...)
		}

	} else {
		if opts.cloneDepth > 0 {
			// If this git log fails, then we know the clone is too shallow so we unshallow before reset.
			gitCommands = append(gitCommands, fmt.Sprintf("git log HEAD..%s || git fetch --unshallow", conf.Task.Revision))
		}
		gitCommands = append(gitCommands, fmt.Sprintf("git reset --hard %s", conf.Task.Revision))
	}

	gitCommands = append(gitCommands, "git log --oneline -n 10")

	return gitCommands, nil
}

func (c *gitFetchProject) buildModuleCloneCommand(conf *internal.TaskConfig, opts cloneOpts, ref string, modulePatch *patch.ModulePatch) ([]string, error) {
	gitCommands := []string{
		"set -o xtrace",
		"set -o errexit",
	}
	if opts.repo == "" && opts.owner == "" {
		return nil, errors.New("must specify owner and repo")
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

func (c *gitFetchProject) opts(cloneToken string, logger client.LoggerProducer, conf *internal.TaskConfig) (cloneOpts, error) {
	shallowCloneEnabled := conf.Distro == nil || !conf.Distro.DisableShallowClone
	opts := cloneOpts{
		owner:             conf.ProjectRef.Owner,
		repo:              conf.ProjectRef.Repo,
		branch:            conf.ProjectRef.Branch,
		dir:               c.Directory,
		token:             cloneToken,
		recurseSubmodules: c.RecurseSubmodules,
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

	if err := opts.validate(); err != nil {
		return opts, errors.Wrap(err, "validating clone options")
	}
	return opts, nil
}

// Execute gets the source code required by the project
// Retries some number of times before failing
func (c *gitFetchProject) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := loadModulesManifestInToExpansions(ctx, comm, conf); err != nil {
		return errors.Wrap(err, "loading manifest")
	}
	logger.Execution().Info("Manifest loaded successfully.")

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	cloneToken, err := getProjectMethodAndToken(ctx, comm, conf, c.Token)
	if err != nil {
		return errors.Wrap(err, "getting method of cloning and token")
	}

	var opts cloneOpts
	opts, err = c.opts(cloneToken, logger, conf)
	if err != nil {
		return err
	}

	err = c.fetch(ctx, comm, logger, conf, td, opts)
	if err != nil {
		logger.Task().Error(message.WrapError(err, message.Fields{
			"operation":    "git.get_project",
			"message":      "cloning failed",
			"num_attempts": gitFetchProjectRetries,
			"owner":        conf.ProjectRef.Owner,
			"repo":         conf.ProjectRef.Repo,
			"branch":       conf.ProjectRef.Branch,
		}))
	}

	return err
}

func (c *gitFetchProject) fetchSource(ctx context.Context, logger client.LoggerProducer, conf *internal.TaskConfig, jpm jasper.Manager, opts cloneOpts) error {
	attempt := 0
	return c.retryFetch(ctx, logger, true, opts, func(opts cloneOpts) error {
		attempt++
		gitCommands, err := c.buildSourceCloneCommand(conf, opts)
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
		fetchSourceCmd := jpm.CreateCommand(ctx).Add([]string{"bash", "-c", fetchScript}).Directory(conf.WorkDir).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Execution().GetSender())

		logger.Execution().Info("Fetching source from git...")
		logger.Execution().Debugf("Commands are: %s", fetchScript)

		ctx, span := getTracer().Start(ctx, "clone_source", trace.WithAttributes(
			attribute.String(cloneOwnerAttribute, opts.owner),
			attribute.String(cloneRepoAttribute, opts.repo),
			attribute.String(cloneBranchAttribute, opts.branch),
			attribute.Int(cloneAttemptAttribute, attempt),
		))
		defer span.End()

		return fetchSourceCmd.Run(ctx)
	})
}

func (c *gitFetchProject) retryFetch(ctx context.Context, logger client.LoggerProducer, isSource bool, opts cloneOpts, fetch func(cloneOpts) error) error {
	const (
		fetchRetryMinDelay = time.Second
		fetchRetryMaxDelay = 10 * time.Second
	)

	fetchType := "module"
	if isSource {
		fetchType = "source"
	}

	var attemptNum int
	return utility.Retry(
		ctx,
		func() (bool, error) {
			if attemptNum > 2 {
				opts.useVerbose = true // use verbose for the last 2 attempts
				logger.Task().Error(message.Fields{
					"message":      fmt.Sprintf("running git '%s' clone with verbose output", fetchType),
					"num_attempts": gitFetchProjectRetries,
					"attempt":      attemptNum,
				})
			}
			if err := fetch(opts); err != nil {
				attemptNum++
				if isSource && attemptNum == 1 {
					logger.Execution().Warning("git source clone failed with cached merge SHA; re-requesting merge SHA from GitHub")
				}
				if strings.Contains(err.Error(), githubMergeQueueInvalidRefError) {
					return false, errors.Wrap(err, "the GitHub merge SHA is not available most likely because the merge completed or was aborted")
				}
				return true, errors.Wrapf(err, "attempt %d", attemptNum)
			}
			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: gitFetchProjectRetries,
			MinDelay:    fetchRetryMinDelay,
			MaxDelay:    fetchRetryMaxDelay,
		})
}

func (c *gitFetchProject) fetchModuleSource(ctx context.Context,
	comm client.Communicator,
	conf *internal.TaskConfig,
	logger client.LoggerProducer,
	jpm jasper.Manager,
	td client.TaskData,
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

	moduleBase := filepath.ToSlash(filepath.Join(conf.ModulePaths[module.Name], module.Name))

	// use submodule revisions based on the main patch. If there is a need in the future,
	// this could maybe use the most recent submodule revision of all requested patches.
	// We ignore set-module changes for commit queue and GitHub merge queue, since we should verify HEAD before merging.
	var modulePatch *patch.ModulePatch
	var revision string
	if p != nil {
		modulePatch := p.FindModule(moduleName)
		if modulePatch != nil {
			if conf.Task.Requester == evergreen.GithubMergeRequester {
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

	opts := cloneOpts{
		branch: "",
		dir:    moduleBase,
	}

	// If the module repo is using the deprecated ssh cloning method, extract the owner
	// and repo from the string and save it to clone options so that the an https cloning link
	// can be constructed manually by opts.setLocation.
	// This is a temporary workaround which will be removed once users have switched over.
	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return errors.Wrapf(err, "getting module owner and repo '%s'", module.Name)
	}

	opts.owner = owner
	opts.repo = repo
	if strings.Contains(module.Repo, "git@github.com:") {
		logger.Task().Warningf("ssh cloning is being deprecated. We are manually converting '%s'"+
			" to https format. Please update your project config.", module.Repo)
	}

	appToken, err := comm.CreateInstallationTokenForClone(ctx, td, opts.owner, opts.repo)
	if err != nil {
		return errors.Wrap(err, "creating app token")
	}

	opts.token = appToken

	// After generating, redact the token from the logs.
	conf.NewExpansions.Redact(generatedTokenKey, appToken)

	if err = opts.validate(); err != nil {
		return errors.Wrap(err, "validating clone options")
	}

	var moduleCmds []string
	moduleCmds, err = c.buildModuleCloneCommand(conf, opts, revision, modulePatch)
	if err != nil {
		return err
	}

	attempt := 0
	return c.retryFetch(ctx, logger, false, opts, func(opts cloneOpts) error {
		attempt++
		ctx, span := getTracer().Start(ctx, "clone_module", trace.WithAttributes(
			attribute.String(cloneModuleAttribute, module.Name),
			attribute.String(cloneOwnerAttribute, opts.owner),
			attribute.String(cloneRepoAttribute, opts.repo),
			attribute.String(cloneBranchAttribute, opts.branch),
			attribute.Int(cloneAttemptAttribute, attempt),
		))
		defer span.End()

		// This needs to use a thread-safe buffer just in case the context errors
		// (e.g. due to a timeout) while the command is running. A non-thread-safe
		// buffer is only safe to read once the command exits, guaranteeing that all
		// output is finished writing. However, if the context errors, Run will
		// return early and will stop waiting for the command to exit. In the
		// context error case, this thread and the still-running command may race to
		// read/write the buffer, so the buffer has to be thread-safe.
		stdOut := utility.MakeSafeBuffer(bytes.Buffer{})
		stdErr := utility.MakeSafeBuffer(bytes.Buffer{})
		err = jpm.CreateCommand(ctx).Add([]string{"bash", "-c", strings.Join(moduleCmds, "\n")}).
			Directory(filepath.ToSlash(GetWorkingDirectory(conf, c.Directory))).
			SetOutputWriter(stdOut).SetErrorWriter(stdErr).Run(ctx)

		// Prefix every line of the output with the module name.
		output := strings.ReplaceAll(stdOut.String(), "\n", fmt.Sprintf("\n%s: ", module.Name))
		logger.Execution().Info(output)

		errOutput := stdErr.String()
		if errOutput != "" {
			errOutput = strings.ReplaceAll(errOutput, "\n", fmt.Sprintf("\n%s: ", module.Name))
			logger.Execution().Error(fmt.Sprintf("%s: %s", module.Name, errOutput))
		}
		return err
	})
}

func (c *gitFetchProject) fetch(ctx context.Context,
	comm client.Communicator,
	logger client.LoggerProducer,
	conf *internal.TaskConfig,
	td client.TaskData,
	opts cloneOpts) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jpm := c.JasperManager()

	// Clone the project.
	if err := c.fetchSource(ctx, logger, conf, jpm, opts); err != nil {
		return errors.Wrap(err, "problem running fetch command")
	}

	// Retrieve the patch for the version if one exists.
	var p *patch.Patch
	var err error
	if evergreen.IsPatchRequester(conf.Task.Requester) {
		logger.Execution().Info("Fetching patch.")
		p, err = comm.GetTaskPatch(ctx, td)
		if err != nil {
			return errors.Wrap(err, "getting patch for task")
		}
	}

	// For every module, expand the module prefix.
	for _, moduleName := range conf.BuildVariant.Modules {
		expanded, err := conf.NewExpansions.ExpandString(moduleName)
		if err == nil {
			moduleName = expanded
		}
		module, err := conf.Project.GetModuleByName(moduleName)
		if err != nil {
			return errors.Wrapf(err, "getting module '%s'", moduleName)
		}
		if module == nil {
			return errors.Errorf("module '%s' not found", moduleName)
		}
		expandModulePrefix(conf, module.Name, module.Prefix, logger)
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	// Clone the project's modules in goroutines.
	for _, name := range conf.BuildVariant.Modules {
		// TODO (DEVPROD-3611): remove capturing the loop variable and use the loop variable directly.
		moduleName := name
		expanded, err := conf.NewExpansions.ExpandString(moduleName)
		if err == nil {
			moduleName = expanded
		}
		g.Go(func() error {
			if err := gCtx.Err(); err != nil {
				return nil
			}
			return errors.Wrapf(c.fetchModuleSource(gCtx, comm, conf, logger, jpm, td, p, moduleName), "fetching module source '%s'", moduleName)
		})
	}

	if err = g.Wait(); err != nil {
		return errors.Wrap(err, "fetching project and module source")
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

	ctx, span := getTracer().Start(ctx, "get_patches")
	defer span.End()

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

// getApplyCommand returns the git apply command
func (c *gitFetchProject) getApplyCommand(patchFile string) (string, error) {
	apply := fmt.Sprintf("GIT_TRACE=1 git apply --binary --index < '%s'", patchFile)
	return apply, nil
}

// getPatchCommands, given a module patch of a patch, will return the appropriate list of commands that
// need to be executed, except for apply. If the patch is empty it will not apply the patch.
func getPatchCommands(modulePatch patch.ModulePatch, moduleDir, patchPath string) []string {
	patchCommands := []string{
		"set -o xtrace",
		"set -o errexit",
	}
	if moduleDir != "" {
		patchCommands = append(patchCommands, fmt.Sprintf("cd '%s'", moduleDir))
	}
	patchCommands = append(patchCommands, fmt.Sprintf("git reset --hard '%s'", modulePatch.Githash))

	if modulePatch.PatchSet.Patch == "" {
		return patchCommands
	}
	return append(patchCommands, fmt.Sprintf("git apply --stat '%v' || true", patchPath))
}

// applyPatch is used by the agent to copy patch data onto disk
// and then call the necessary git commands to apply the patch file
func (c *gitFetchProject) applyPatch(ctx context.Context, logger client.LoggerProducer,
	conf *internal.TaskConfig, patches []patch.ModulePatch) error {

	ctx, span := getTracer().Start(ctx, "apply_patches")
	defer span.End()

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
			expandModulePrefix(conf, module.Name, module.Prefix, logger)
			moduleDir = filepath.ToSlash(filepath.Join(conf.ModulePaths[module.Name], module.Name))
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
		tempFile, err := os.CreateTemp("", "mcipatch_")
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() { //nolint:evg-lint
			grip.Error(tempFile.Close())
			grip.Error(os.Remove(tempFile.Name()))
		}()
		_, err = io.WriteString(tempFile, patchPart.PatchSet.Patch)
		if err != nil {
			return errors.WithStack(err)
		}
		tempAbsPath := tempFile.Name()

		// this applies the patch using the patch files in the temp directory
		patchCommandStrings := getPatchCommands(patchPart, moduleDir, tempAbsPath)
		applyCommand, err := c.getApplyCommand(tempAbsPath)
		if err != nil {
			return errors.Wrap(err, "getting git apply command")
		}
		patchCommandStrings = append(patchCommandStrings, applyCommand)
		cmdsJoined := strings.Join(patchCommandStrings, "\n")

		cmd := jpm.CreateCommand(ctx).Add([]string{"bash", "-c", cmdsJoined}).
			Directory(filepath.ToSlash(GetWorkingDirectory(conf, c.Directory))).
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
	return conf.GithubPatchData.PRNumber != 0 || conf.GithubMergeData.HeadSHA != ""
}

type noopWriteCloser struct {
	*bytes.Buffer
}

func (noopWriteCloser) Close() error { return nil }
