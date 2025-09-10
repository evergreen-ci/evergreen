package command

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
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

	// Logging templates.
	moduleRevision    = "Using revision/ref '%s' for module '%s' (reason: %s)."
	sshCloningWarning = "ssh cloning is being deprecated. We are manually converting '%s' to https format. Please update your project config."

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

type gitFetchProject2 struct {
	// The root directory (locally) that the code should be checked out into.
	// Must be a valid non-blank directory name.
	Directory string `plugin:"expand"`

	// Revisions are the optional revisions associated with the modules of a project.
	// Note: If a module does not have a revision it will use the module's branch to get the project.
	Revisions map[string]string `plugin:"expand"`

	Token string `plugin:"expand" mapstructure:"token"`

	// ShallowClone sets CloneDepth to 100, and is kept for backwards compatibility.
	ShallowClone bool `mapstructure:"shallow_clone"`
	// CloneDepth sets the depth of the clone operation. If 0, performs a full clone.
	CloneDepth int `mapstructure:"clone_depth"`

	RecurseSubmodules bool `mapstructure:"recurse_submodules"`

	CommitterName  string `mapstructure:"committer_name"`
	CommitterEmail string `mapstructure:"committer_email"`

	// These are captured when Execute is called.
	comm   client.Communicator
	logger client.LoggerProducer
	conf   *internal.TaskConfig

	patch *patch.Patch

	base
}

func gitFetchProjectFactory2() Command   { return &gitFetchProject2{} }
func (c *gitFetchProject2) Name() string { return "git.get_project2" }

// ParseParams parses the command's configuration.
// Fulfills the Command interface.
func (c *gitFetchProject2) ParseParams(params map[string]any) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	if c.Directory == "" {
		return errors.New("directory must not be blank")
	}

	if c.ShallowClone && c.CloneDepth == 0 {
		c.CloneDepth = shallowCloneDepth
	}

	return nil
}

func (c *gitFetchProject2) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	// Load the modules manfiest into the expansions to allow them as
	// as expansions when expanding the parameters.
	if err := loadModulesManifestInToExpansions(ctx, comm, conf); err != nil {
		return errors.Wrap(err, "loading manifest")
	}
	logger.Execution().Info("Manifest loaded successfully.")

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	c.comm = comm
	c.logger = logger
	c.conf = conf

	// Concurrently clone the main repository and fetch the patch (if applicable).
	// This is because these operations are independent and may take some time each.
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := c.cloneSource(gCtx); err != nil {
			return errors.Wrap(err, "cloning main repository")
		}
		return nil
	})
	// If this is a patch task, fetch the patch.
	// This is used for patch-only features, like module patches.
	// And it's used to determine the module revision(s) to check out.
	if evergreen.IsPatchRequester(conf.Task.Requester) {
		g.Go(func() error {
			logger.Execution().Info("Fetching patch.")
			var err error
			if c.patch, err = comm.GetTaskPatch(gCtx, conf.TaskData()); err != nil {
				return errors.Wrap(err, "getting patch for task")
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Concurrently clone modules and apply patches to the main repository.
	// This is because these operations are independent and may take some time each.
	g, gCtx = errgroup.WithContext(ctx)
	g.Go(func() error {
		// Before cloning modules, we expand the module names and prefixes.
		if err := expandModuleNamesAndPrefixes(conf, logger); err != nil {
			return errors.Wrap(err, "expanding module names and prefixes")
		}

		return errors.Wrap(c.cloneModules(gCtx), "cloning modules")
	})
	if c.patch != nil && len(c.patch.Patches) > 0 {
		g.Go(func() error {
			return errors.Wrap(c.applySourcePatches(gCtx), "applying patches to main repository")
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (c *gitFetchProject2) cloneSource(ctx context.Context) error {
	ctx, span := getTracer().Start(ctx, "clone_source")
	defer span.End()

	opts, err := c.sourceCloneOptions(ctx)
	if err != nil {
		return errors.Wrap(err, "getting source clone options")
	}

	attempt := 0
	return c.retryFetch(ctx, true, opts, func(ctx context.Context, script string) error {
		attempt++
		// This needs to use a thread-safe buffer just in case the context errors
		// (e.g. due to a timeout) while the command is running. A non-thread-safe
		// buffer is only safe to read once the command exits, guaranteeing that all
		// output is finished writing. However, if the context errors, Run will
		// return early and will stop waiting for the command to exit. In the
		// context error case, this thread and the still-running command may race to
		// read/write the buffer, so the buffer has to be thread-safe.
		stdOut := utility.MakeSafeBuffer(bytes.Buffer{})
		stdErr := utility.MakeSafeBuffer(bytes.Buffer{})
		fetchSourceCmd := c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", script}).Directory(c.conf.WorkDir).
			SetOutputWriter(stdOut).SetErrorWriter(stdErr)

		c.logger.Execution().Info("Fetching source from git...")
		c.logger.Execution().Debugf("Commands are: %s", script)

		err := fetchSourceCmd.Run(ctx)

		c.logger.Execution().Info(stdOut.String())
		c.logger.Execution().Error(stdErr.String())

		return err
	})
}

func (c *gitFetchProject2) sourceCloneOptions(ctx context.Context) (*cloneCMDOptions, error) {
	cloneToken, err := getCloneToken(ctx, c.comm, c.conf, c.Token)
	if err != nil {
		return nil, errors.Wrap(err, "getting method of cloning and token")
	}

	depth := 0
	// If the distro has disabled shallow clones, override any depth settings to perform a full clone.
	if c.conf.DistroDisablesShallowClone() {
		c.logger.Task().Infof("Clone depth is disabled for this distro; ignoring-user specified clone depth.")
	} else {
		// Otherwise, use the user-specified depth settings.
		if c.CloneDepth > 0 {
			depth = c.CloneDepth
			if c.ShallowClone {
				c.logger.Task().Infof("Specified clone depth of %d will be used instead of shallow_clone (which uses depth %d).", depth, shallowCloneDepth)
			}
		} else if c.ShallowClone {
			depth = shallowCloneDepth
		}
	}

	return &cloneCMDOptions{
		conf:              c.conf,
		owner:             c.conf.ProjectRef.Owner,
		repo:              c.conf.ProjectRef.Repo,
		dir:               c.Directory,
		branch:            c.conf.ProjectRef.Branch,
		recurseSubmodules: c.RecurseSubmodules,
		token:             cloneToken,
		depth:             depth,
	}, nil
}

func (c *gitFetchProject2) cloneModules(ctx context.Context) error {
	ctx, span := getTracer().Start(ctx, "clone_modules")
	defer span.End()

	var modules []*model.Module
	for moduleName, _ := range c.conf.ModulePaths {
		module, err := c.conf.Project.GetModuleByName(moduleName)
		if err != nil {
			return errors.Wrapf(err, "getting module '%s'", moduleName)
		}
		modules = append(modules, module)
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(10) // Limit to 10 concurrent module clones and patch applies.
	for _, module := range modules {
		g.Go(func() error {
			if err := c.cloneModule(gCtx, module); err != nil {
				return errors.Wrap(err, "cloning module")
			}
			if c.patch != nil && len(c.patch.Patches) > 0 {
				if err := c.applyModulePatches(gCtx, module); err != nil {
					return errors.Wrap(err, "applying patches to module")
				}
			}
			return nil
		})
	}
	return g.Wait()
}

func (c *gitFetchProject2) cloneModule(ctx context.Context, module *model.Module) error {
	opts, err := c.moduleCloneOptions(ctx, module)
	if err != nil {
		return errors.Wrap(err, "getting module clone options")
	}

	return c.retryFetch(ctx, true, opts, func(ctx context.Context, script string) error {
		trace.SpanFromContext(ctx).SetAttributes(
			attribute.String(cloneModuleAttribute, module.Name),
		)
		// This needs to use a thread-safe buffer just in case the context errors
		// (e.g. due to a timeout) while the command is running. A non-thread-safe
		// buffer is only safe to read once the command exits, guaranteeing that all
		// output is finished writing. However, if the context errors, Run will
		// return early and will stop waiting for the command to exit. In the
		// context error case, this thread and the still-running command may race to
		// read/write the buffer, so the buffer has to be thread-safe.
		stdOut := utility.MakeSafeBuffer(bytes.Buffer{})
		stdErr := utility.MakeSafeBuffer(bytes.Buffer{})
		err := c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", script}).Directory(filepath.ToSlash(GetWorkingDirectory(c.conf, c.Directory))).
			SetOutputWriter(stdOut).SetErrorWriter(stdErr).Run(ctx)

		// Prefix every line of the output with the module name.
		output := strings.ReplaceAll(stdOut.String(), "\n", fmt.Sprintf("\n%s: ", module.Name))
		c.logger.Execution().Info(output)

		errOutput := stdErr.String()
		if errOutput != "" {
			errOutput = strings.ReplaceAll(errOutput, "\n", fmt.Sprintf("\n%s: ", module.Name))
			c.logger.Execution().Error(fmt.Sprintf("%s: %s", module.Name, errOutput))
		}

		return err
	})
}

func (c *gitFetchProject2) moduleCloneOptions(ctx context.Context, module *model.Module) (*cloneCMDOptions, error) {
	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "getting module owner and repo '%s'", module.Name)
	}
	if strings.Contains(module.Repo, "git@github.com:") {
		c.logger.Task().Warningf(sshCloningWarning, module.Repo)
	}

	cloneToken, err := c.comm.CreateInstallationTokenForClone(ctx, c.conf.TaskData(), owner, repo)
	if err != nil {
		return nil, errors.Wrap(err, "creating app token")
	}

	return &cloneCMDOptions{
		conf:  c.conf,
		owner: owner,
		repo:  repo,
		dir:   filepath.ToSlash(filepath.Join(c.conf.ModulePaths[module.Name], module.Name)),
		token: cloneToken,
		ref:   c.getModuleRevision(module),
	}, nil
}

func (c *gitFetchProject2) getModuleRevision(module *model.Module) string {
	// First, try to get the revision from the module patch.
	if c.patch != nil {
		modulePatch := c.patch.FindModule(module.Name)
		if modulePatch != nil {
			// If this is commit queue, default to HEAD (since before merging
			// we should test against HEAD).
			if c.conf.Task.Requester == evergreen.GithubMergeRequester {
				if module.Branch != "" {
					c.logger.Execution().Infof(moduleRevision, module.Branch, module.Name, "defaulting to HEAD for merge")
					return module.Branch
				}
			}
			// Otherwise, use the revision from the module patch if provided.
			if gitHash := modulePatch.Githash; gitHash != "" {
				c.logger.Execution().Infof(moduleRevision, modulePatch.Githash, module.Name, "specified in set-module")
				return modulePatch.Githash
			}
		}
	}

	// Next, try to get the revision from the command parameters.
	if cmdParam := c.Revisions[module.Name]; cmdParam != "" {
		c.logger.Execution().Infof(moduleRevision, cmdParam, module.Name, "specified as parameter to git.get_project")
		return cmdParam
	}

	// Next, try to get the revision from an expansion.
	if exp := c.conf.Expansions.Get(moduleRevExpansionName(module.Name)); exp != "" {
		c.logger.Execution().Infof(moduleRevision, exp, module.Name, "from manifest")
		return exp
	}

	if module.Ref != "" {
		c.logger.Execution().Infof(moduleRevision, module.Ref, module.Name, "ref field in config file")
		return module.Ref
	}

	return ""
}

func (c *gitFetchProject2) applySourcePatches(ctx context.Context) error {
	ctx, span := getTracer().Start(ctx, "apply_source_patches")
	defer span.End()

	// Patches for the source are stored alongside module patches.
	// To apply only the source ones, we filter out the module patches.
	for _, modulePatch := range c.patch.Patches {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while applying source patches")
		}
		if modulePatch.ModuleName != "" {
			// skip module patches
			continue
		}
		if modulePatch.PatchSet.PatchFileId != "" {
			var err error
			modulePatch.PatchSet.Patch, err = c.comm.GetPatchFile(ctx, c.conf.TaskData(), modulePatch.PatchSet.PatchFileId)
			if err != nil {
				return errors.Wrapf(err, "getting patch file")
			}
		}
		c.logger.Execution().Info("Applying source patch with git...")
		if err := c.applyPatch(ctx, &modulePatch); err != nil {
			return errors.Wrapf(err, "applying patch to source")
		}
	}

	return nil
}

func (c *gitFetchProject2) applyModulePatches(ctx context.Context, module *model.Module) error {
	ctx, span := getTracer().Start(ctx, "apply_module_patches")
	defer span.End()

	// Patches for the source are stored alongside module patches.
	// To apply only the source ones, we filter out the module patches.
	for _, modulePatch := range c.patch.Patches {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "canceled while applying source patches")
		}
		if modulePatch.ModuleName != module.Name {
			// skip other module patches
			continue
		}
		if modulePatch.PatchSet.PatchFileId != "" {
			var err error
			modulePatch.PatchSet.Patch, err = c.comm.GetPatchFile(ctx, c.conf.TaskData(), modulePatch.PatchSet.PatchFileId)
			if err != nil {
				return errors.Wrapf(err, "getting patch file")
			}
		}
		c.logger.Execution().Info("Applying source patch with git...")
		if err := c.applyPatch(ctx, &modulePatch); err != nil {
			return errors.Wrapf(err, "applying patch to source")
		}
	}

	return nil
}

func (c *gitFetchProject2) applyPatch(ctx context.Context, modulePatch *patch.ModulePatch) error {
	if len(modulePatch.PatchSet.Patch) == 0 {
		c.logger.Execution().Info("Skipping empty patch file...")
		return nil
	}
	dir := GetWorkingDirectory(c.conf, c.Directory)
	if moduleName := modulePatch.ModuleName; moduleName != "" {
		dir = filepath.Join(dir, c.conf.ModulePaths[moduleName], moduleName)
	}
	dir = filepath.ToSlash(dir)

	cmds := []string{
		"set -o xtrace",
		"set -o errexit",
	}

	dryRun := strings.Join(append(cmds, "git apply --stat || true"), "\n")
	cmd := c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", dryRun}).
		Directory(dir).
		SetOutputSender(level.Info, c.logger.Task().GetSender()).SetErrorSender(level.Error, c.logger.Task().GetSender()).SetInputBytes([]byte(modulePatch.PatchSet.Patch))
	if err := cmd.Run(ctx); err != nil {
		return errors.Wrap(err, "running dryrun git apply")
	}

	applyCommand := "GIT_TRACE=1 git apply --binary --index"
	applyPatch := strings.Join(append(cmds, applyCommand), "\n")
	cmd = c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", applyPatch}).
		Directory(dir).
		SetOutputSender(level.Info, c.logger.Task().GetSender()).SetErrorSender(level.Error, c.logger.Task().GetSender()).SetInputBytes([]byte(modulePatch.PatchSet.Patch))
	if err := cmd.Run(ctx); err != nil {
		return errors.Wrap(err, "running git apply")
	}

	return nil
}

func (c *gitFetchProject2) retryFetch(ctx context.Context, isSource bool, opts *cloneCMDOptions, fetch func(context.Context, string) error) error {
	const (
		fetchRetryMinDelay = time.Second
		fetchRetryMaxDelay = 10 * time.Second
	)

	fetchType := "module"
	if isSource {
		fetchType = "source"
	}

	attempt := 0
	return utility.Retry(
		ctx,
		func() (bool, error) {
			attempt++

			spanName := "retry_clone_source"
			if !isSource {
				spanName = "retry_clone_module"
			}
			ctx, span := getTracer().Start(ctx, spanName, trace.WithAttributes(
				attribute.String(cloneOwnerAttribute, opts.owner),
				attribute.String(cloneRepoAttribute, opts.repo),
				attribute.String(cloneBranchAttribute, opts.branch),
				attribute.Int(cloneAttemptAttribute, attempt),
			))
			defer span.End()

			// Use verbose for the last 2 attempts.
			if attempt > 3 {
				opts.useVerbose = true
				c.logger.Task().Error(message.Fields{
					"message":      fmt.Sprintf("running git '%s' clone with verbose output", fetchType),
					"num_attempts": gitFetchProjectRetries,
					"attempt":      attempt,
				})
			}

			gitCommands, err := opts.build()
			if err != nil {
				return false, errors.Wrap(err, "building git commands")
			}
			if err := fetch(ctx, strings.Join(gitCommands, "\n")); err != nil {
				if isSource && attempt == 1 {
					c.logger.Execution().Warning("git source clone failed with cached merge SHA; re-requesting merge SHA from GitHub")
				}
				if strings.Contains(err.Error(), githubMergeQueueInvalidRefError) {
					return false, errors.Wrap(err, "the GitHub merge SHA is not available most likely because the merge completed or was aborted")
				}
				return true, errors.Wrapf(err, "attempt %d", attempt)
			}
			return false, nil
		}, utility.RetryOptions{
			MaxAttempts: gitFetchProjectRetries,
			MinDelay:    fetchRetryMinDelay,
			MaxDelay:    fetchRetryMaxDelay,
		})
}

func moduleRevExpansionName(name string) string { return fmt.Sprintf("%s_rev", name) }

type cloneCMDOptions struct {
	// Required fields.
	owner, repo, dir, token string
	conf                    *internal.TaskConfig

	// Optional fields.
	useVerbose bool

	// Source-specific fields.
	branch            string
	depth             int
	recurseSubmodules bool

	// Module-specific fields.
	// ref is the module revision or branch to check out after cloning.
	ref         string
	modulePatch *patch.ModulePatch
}

func (opts *cloneCMDOptions) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.owner == "", "missing required owner")
	catcher.NewWhen(opts.repo == "", "missing required repo")
	catcher.NewWhen(opts.dir == "", "missing required directory")
	catcher.NewWhen(opts.token == "", "missing required token")
	catcher.NewWhen(opts.depth < 0, "clone depth cannot be negative")

	// If this is a module clone, we require a ref to be specified unless
	// this is a GitHub PR module patch (in which case we default to HEAD).
	if opts.modulePatch != nil && opts.ref == "" && !isGitHubPRModulePatch(opts.conf, opts.modulePatch) {
		catcher.New("missing required ref for module patch")
	}

	return catcher.Resolve()
}

func (opts *cloneCMDOptions) build() ([]string, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid clone command options")
	}

	cmds := []string{
		"set +o xtrace", // disable xtrace for the echo of the git clone command
	}

	gitURL := thirdparty.FormGitURLForApp(opts.owner, opts.repo, opts.token)
	clone := fmt.Sprintf("git clone %s '%s'", gitURL, opts.dir)

	if opts.recurseSubmodules {
		clone = fmt.Sprintf("%s --recurse-submodules", clone)
	}
	if opts.useVerbose {
		clone = fmt.Sprintf("GIT_TRACE=1 %s", clone)
	}
	if opts.depth > 0 {
		clone = fmt.Sprintf("%s --depth %d", clone, opts.depth)
	}
	if opts.branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, opts.branch)
	}
	cmds = append(cmds, clone)

	// Source-specific post-clone commands.
	if opts.modulePatch == nil {
		// TODO-zackary: Refactor this
		if isGitHub(opts.conf) {
			var suffix, localBranchName, remoteBranchName, commitToTest string
			if opts.conf.Task.Requester == evergreen.GithubPRRequester {
				// Github creates a ref called refs/pull/[pr number]/head
				// that provides the entire tree of changes, including merges
				suffix = "/head"
				commitToTest = opts.conf.GithubPatchData.HeadHash
				localBranchName = fmt.Sprintf("evg-pr-test-%s", utility.RandomString())
				remoteBranchName = fmt.Sprintf("pull/%d", opts.conf.GithubPatchData.PRNumber)
			} else if opts.conf.Task.Requester == evergreen.GithubMergeRequester {
				suffix = "" // redundant, included for clarity
				commitToTest = opts.conf.GithubMergeData.HeadSHA
				localBranchName = fmt.Sprintf("evg-mg-test-%s", utility.RandomString())
				// HeadRef looks like "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
				remoteBranchName = opts.conf.GithubMergeData.HeadBranch
			}
			if commitToTest != "" {
				cmds = append(cmds, []string{
					fmt.Sprintf(`git fetch origin "%s%s:%s"`, remoteBranchName, suffix, localBranchName),
					fmt.Sprintf(`git checkout "%s"`, localBranchName),
					fmt.Sprintf("git reset --hard %s", commitToTest),
				}...)
			}

		} else {
			if opts.depth > 0 {
				// If this git log fails, then we know the clone is too shallow so we unshallow before reset.
				cmds = append(cmds, fmt.Sprintf("git log HEAD..%s || git fetch --unshallow", opts.conf.Task.Revision))
			}
			cmds = append(cmds, fmt.Sprintf("git reset --hard %s", opts.conf.Task.Revision))
		}
	}

	// Module-specific post-clone commands.
	if opts.modulePatch != nil {
		if isGitHubPRModulePatch(opts.conf, opts.modulePatch) {
			branchName := fmt.Sprintf("evg-merge-test-%s", utility.RandomString())
			cmds = append(cmds,
				fmt.Sprintf(`git fetch origin "pull/%s/merge:%s"`, opts.modulePatch.PatchSet.Patch, branchName),
				fmt.Sprintf("git checkout '%s'", branchName),
				fmt.Sprintf("git reset --hard %s", opts.modulePatch.Githash),
			)
		} else {
			cmds = append(cmds, fmt.Sprintf("git checkout '%s'", opts.ref))
		}
	}

	return append(
		cmds,
		"set -o xtrace",
		fmt.Sprintf("cd %s", opts.dir),
	), nil
}

func isGitHubPRModulePatch(conf *internal.TaskConfig, modulePatch *patch.ModulePatch) bool {
	patchProvided := (modulePatch != nil) && (modulePatch.PatchSet.Patch != "")
	return isGitHub(conf) && patchProvided
}

func isGitHub(conf *internal.TaskConfig) bool {
	return conf.GithubPatchData.PRNumber != 0 || conf.GithubMergeData.HeadSHA != ""
}

func loadModulesManifestInToExpansions(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig) error {
	manifest, err := comm.GetManifest(ctx, conf.TaskData())
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
