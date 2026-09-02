package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

	// restoreSubmoduleCredentialsRewrite injects the task's token into scrubbed GitHub submodule remotes.
	restoreSubmoduleCredentialsRewrite = `s#://github\.com/#://x-access-token:%s@github.com/#g`
)

var (
	cloneOwnerAttribute   = fmt.Sprintf("%s.clone_owner", gitGetProjectAttribute)
	cloneRepoAttribute    = fmt.Sprintf("%s.clone_repo", gitGetProjectAttribute)
	cloneBranchAttribute  = fmt.Sprintf("%s.clone_branch", gitGetProjectAttribute)
	cloneModuleAttribute  = fmt.Sprintf("%s.clone_module", gitGetProjectAttribute)
	cloneAttemptAttribute = fmt.Sprintf("%s.attempt", gitGetProjectAttribute)
	cloneErrorAttribute   = fmt.Sprintf("%s.error", gitGetProjectAttribute)
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

	// Filter passes git clone's --filter (partial clone), e.g. "blob:none" to
	// omit file content from the initial clone and fetch blobs lazily on demand.
	Filter string `plugin:"expand" mapstructure:"filter"`

	// SparseCheckoutPaths restricts the working tree to the listed paths via
	// git sparse-checkout (no-cone mode). Only takes effect when Filter is set;
	// otherwise the paths are ignored and a normal full clone runs.
	//
	// Entries are gitignore-style patterns, NOT plain paths: an unanchored entry
	// like "README.md" matches that name in every directory. To select one
	// specific file, anchor it with a leading slash, e.g. "/scripts/foo.sh".
	SparseCheckoutPaths []string `plugin:"expand" mapstructure:"sparse_checkout_paths"`

	RecurseSubmodules bool `mapstructure:"recurse_submodules"`

	CommitterName string `mapstructure:"committer_name"`

	CommitterEmail string `mapstructure:"committer_email"`

	refNotFound bool

	base
}

type cloneOpts struct {
	owner               string
	repo                string
	branch              string
	dir                 string
	token               string
	recurseSubmodules   bool
	useVerbose          bool
	cloneDepth          int
	filter              string
	sparseCheckoutPaths []string
}

func (opts cloneOpts) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.owner == "", "missing required owner")
	catcher.NewWhen(opts.repo == "", "missing required repo")

	catcher.NewWhen(opts.cloneDepth < 0, "clone depth cannot be negative")
	return catcher.Resolve()
}

// getCloneToken returns the project's clone method and token. If
// set, the project token takes precedence over GitHub App token which takes precedence over over global settings.
func getCloneToken(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, providedToken string) (string, error) {
	if providedToken != "" {
		token, err := parseToken(providedToken)
		return token, err
	}

	owner := conf.ProjectRef.Owner
	repo := conf.ProjectRef.Repo
	appToken, err := comm.CreateInstallationTokenForClone(ctx, conf.TaskData(), owner, repo, false)
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

	// A sparse checkout without a partial-clone filter still downloads every
	// blob, so an empty filter means the sparse paths are ignored.
	if opts.filter == "" {
		opts.sparseCheckoutPaths = nil
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
	if opts.filter != "" {
		clone = fmt.Sprintf("%s --filter=%s", clone, util.ShellQuote(opts.filter))
	}
	if len(opts.sparseCheckoutPaths) > 0 {
		// --no-checkout defers populating the tree until after the sparse set is
		// applied below, so the default sparse contents are never materialized.
		clone = fmt.Sprintf("%s --sparse --no-checkout", clone)
	}
	if opts.branch != "" {
		clone = fmt.Sprintf("%s --branch '%s'", clone, opts.branch)
	}

	cmds := []string{
		"set +o xtrace",
		fmt.Sprintf(`echo %s`, strconv.Quote(clone)),
		clone,
		"set -o xtrace",
		fmt.Sprintf("cd %s", opts.dir),
	}

	if len(opts.sparseCheckoutPaths) > 0 {
		// no-cone mode (git 2.35+) treats the entries as gitignore-style
		// patterns (see the SparseCheckoutPaths field doc on anchoring).
		quotedPaths := make([]string, len(opts.sparseCheckoutPaths))
		for i, p := range opts.sparseCheckoutPaths {
			quotedPaths[i] = util.ShellQuote(p)
		}
		sparse := fmt.Sprintf("git sparse-checkout set --no-cone %s",
			strings.Join(quotedPaths, " "))
		cmds = append(cmds, sparse)
	}

	return cmds, nil
}

// getCloneCommandForWikiModule runs a plain git clone to opts.dir. GitHub
// wikis are cloned at the remote default branch (HEAD) only; branch, ref,
// depth, and submodules are not used.
func (opts cloneOpts) getCloneCommandForWikiModule() ([]string, error) {
	if err := opts.validate(); err != nil {
		return nil, errors.Wrap(err, "invalid clone command options")
	}

	gitURL := thirdparty.FormGitURLForApp(opts.owner, opts.repo, opts.token)
	clone := fmt.Sprintf("git clone %s '%s'", gitURL, opts.dir)
	if opts.useVerbose {
		clone = fmt.Sprintf("GIT_TRACE=1 %s", clone)
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

	// If there's a PR checkout the ref containing the changes.
	if usesGitHubParentPRCheckout(conf) {
		gitCommands = append(gitCommands, prCheckoutCommands(conf)...)
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

// prCheckoutCommit returns the commit a PR or merge queue checkout leaves HEAD
// at, or "" when the task does no PR checkout.
func prCheckoutCommit(conf *internal.TaskConfig) string {
	if conf.GitHubParentPRCheckout != nil && conf.GitHubParentPRCheckout.ForSource {
		return conf.GitHubParentPRCheckout.HeadHash
	}
	switch conf.Task.Requester {
	case evergreen.GithubPRRequester:
		return conf.GithubPatchData.HeadHash
	case evergreen.GithubMergeRequester:
		return conf.GithubMergeData.HeadSHA
	}
	return ""
}

// prCheckoutCommands returns the commands that check out the ref containing a
// PR's or merge queue group's changes. They run after a clone, which leaves HEAD
// at the base revision.
func prCheckoutCommands(conf *internal.TaskConfig) []string {
	commitToTest := prCheckoutCommit(conf)
	if commitToTest == "" {
		return nil
	}

	var suffix, localBranchName, remoteBranchName string
	if conf.GitHubParentPRCheckout != nil && conf.GitHubParentPRCheckout.ForSource {
		suffix = "/head"
		localBranchName = fmt.Sprintf("evg-pr-test-%s", utility.RandomString())
		remoteBranchName = fmt.Sprintf("pull/%d", conf.GitHubParentPRCheckout.PRNumber)
	} else if conf.Task.Requester == evergreen.GithubPRRequester {
		// Github creates a ref called refs/pull/[pr number]/head
		// that provides the entire tree of changes, including merges
		suffix = "/head"
		localBranchName = fmt.Sprintf("evg-pr-test-%s", utility.RandomString())
		remoteBranchName = fmt.Sprintf("pull/%d", conf.GithubPatchData.PRNumber)
	} else if conf.Task.Requester == evergreen.GithubMergeRequester {
		suffix = "" // redundant, included for clarity
		localBranchName = fmt.Sprintf("evg-mg-test-%s", utility.RandomString())
		// HeadRef looks like "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
		remoteBranchName = conf.GithubMergeData.HeadBranch
	}

	return []string{
		fmt.Sprintf(`git fetch origin "%s%s:%s"`, remoteBranchName, suffix, localBranchName),
		fmt.Sprintf(`git checkout "%s"`, localBranchName),
		fmt.Sprintf("git reset --hard %s", commitToTest),
	}
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
	if model.IsWikiRepo(opts.repo) {
		cloneCmd, err := opts.getCloneCommandForWikiModule()
		if err != nil {
			return nil, errors.Wrap(err, "getting command to clone wiki")
		}
		gitCommands = append(gitCommands, cloneCmd...)
		return gitCommands, nil
	}

	if ref == "" && !moduleUsesGitHubParentPRCheckout(conf, modulePatch) {
		return nil, errors.New("empty ref/branch to check out")
	}

	cloneCmd, err := opts.getCloneCommand()
	if err != nil {
		return nil, errors.Wrap(err, "getting command to clone repo")
	}
	gitCommands = append(gitCommands, cloneCmd...)

	if moduleUsesGitHubParentPRCheckout(conf, modulePatch) {
		checkout := conf.GitHubParentPRCheckout
		branchName := fmt.Sprintf("evg-pr-test-%s", utility.RandomString())
		gitCommands = append(gitCommands, []string{
			fmt.Sprintf(`git fetch origin "pull/%d/head:%s"`, checkout.PRNumber, branchName),
			fmt.Sprintf(`git checkout "%s"`, branchName),
			fmt.Sprintf("git reset --hard %s", checkout.HeadHash),
		}...)
	} else {
		gitCommands = append(gitCommands, fmt.Sprintf("git checkout '%s'", ref))
	}

	return gitCommands, nil
}

func (c *gitFetchProject) opts(ctx context.Context, cloneToken string, logger client.LoggerProducer, conf *internal.TaskConfig) (cloneOpts, error) {
	shallowCloneEnabled := conf.Distro == nil || !conf.Distro.DisableShallowClone
	opts := cloneOpts{
		owner:               conf.ProjectRef.Owner,
		repo:                conf.ProjectRef.Repo,
		branch:              conf.ProjectRef.Branch,
		dir:                 c.Directory,
		token:               cloneToken,
		recurseSubmodules:   c.RecurseSubmodules,
		filter:              c.Filter,
		sparseCheckoutPaths: c.SparseCheckoutPaths,
	}

	cloneDepth := c.CloneDepth
	if cloneDepth == 0 && c.ShallowClone {
		// Experiments with shallow clone on AWS hosts suggest that depth 100 is as fast as 1, but 1000 is slower.
		cloneDepth = shallowCloneDepth
	}
	if !shallowCloneEnabled && cloneDepth != 0 {
		logger.Task().Infof(ctx, "Clone depth is disabled for this distro; ignoring-user specified clone depth.")
	} else {
		opts.cloneDepth = cloneDepth
		if c.CloneDepth != 0 && c.ShallowClone {
			logger.Task().Infof(ctx, "Specified clone depth of %d will be used instead of shallow_clone (which uses depth %d).", opts.cloneDepth, shallowCloneDepth)
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
	logger.Task().Info(ctx, "Manifest loaded successfully.")

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	cloneToken, err := getCloneToken(ctx, comm, conf, c.Token)
	if err != nil {
		return errors.Wrap(err, "getting method of cloning and token")
	}

	var opts cloneOpts
	opts, err = c.opts(ctx, cloneToken, logger, conf)
	if err != nil {
		return err
	}

	err = c.fetch(ctx, comm, logger, conf, opts)
	if err != nil {
		logger.Task().Error(ctx, message.WrapError(err, message.Fields{
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

// fetchOrRestoreSource restores the project directory from the source cache when
// the project is opted in, and otherwise clones it from GitHub. Any source cache
// failure falls back to the clone, so the cache can only save time, never change
// what is tested.
func (c *gitFetchProject) fetchOrRestoreSource(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, opts cloneOpts) error {
	sc, skipReason := newSourceCache(conf, c, opts, runtime.GOOS)
	if sc == nil {
		logger.Task().Infof(ctx, "Not using the source cache: %s.", skipReason)
		setSourceCacheSpanSkipped(ctx, skipReason)
		_, err := c.cloneSource(ctx, comm, logger, conf, opts)
		return err
	}

	// A cache hit on the merge SHA never contacts GitHub, so nothing else would notice a deleted ref.
	deleted, err := mergeQueueRefDeleted(ctx, comm, conf, c.Token, opts)
	if err != nil {
		// The clone's own check is still a backstop for an inconclusive answer.
		logger.Task().Warningf(ctx, "Checking whether the merge queue ref still exists: %s", err)
	} else if deleted {
		c.refNotFound = true
		if markErr := comm.MarkMergeQueueGitRefNotFound(ctx, conf.TaskData()); markErr != nil {
			logger.Task().Warningf(ctx, "Failed to mark git ref not found: %s", markErr)
		}
		setSourceCacheSpanSkipped(ctx, "the merge queue ref was deleted")
		return errors.New(mergeQueueRefGoneMessage)
	}

	fallbackReason := ""
	for _, entry := range sc.entries {
		revision := entry.revision
		logger.Task().Infof(ctx, "Looking up source cache '%s/%s'.", sc.cfg.Name, entry.remoteKey)
		restored, err := sc.restore(ctx, comm, logger, entry.remoteKey)
		if err != nil {
			fallbackReason = err.Error()
			break
		}
		if !restored {
			continue
		}
		// A base revision artifact is at the commit the PR is built on, so the
		// PR still has to be checked out over it.
		runPRCheckout := revision != sc.revision
		// buildPostRestoreCommand verifies HEAD against revision, so a restored
		// tree at the wrong commit fails here and falls back to a clone below.
		if err := c.runCommands(ctx, logger, conf, c.buildPostRestoreCommand(conf, opts, revision, runPRCheckout)); err != nil {
			fallbackReason = errors.Wrap(err, "preparing restored source tree").Error()
			break
		}
		logger.Task().Infof(ctx, "Restored source from the cache for revision '%s'.", revision)
		trace.SpanFromContext(ctx).SetAttributes(attribute.String(sourceCacheRestoredRevisionAttribute, revision))
		sc.setSpanOutcome(ctx, sourceCacheHit, "")
		return nil
	}
	if fallbackReason != "" {
		logger.Task().Warningf(ctx, "Falling back to a GitHub clone: %s", fallbackReason)
	}

	token, err := c.cloneSource(ctx, comm, logger, conf, opts)
	if err != nil {
		return err
	}
	// The clone may have refreshed the token, and the post-save script puts this
	// URL back into .git for the rest of the task to use.
	opts.token = token

	outcome := sourceCacheMissProduced
	produced, err := c.saveSourceCache(ctx, comm, logger, conf, sc, opts)
	if err != nil {
		logger.Task().Warningf(ctx, "Saving source to the cache: %s", err)
		if fallbackReason == "" {
			fallbackReason = err.Error()
		}
	} else if !produced {
		outcome = sourceCacheMissLostRace
	}
	if fallbackReason != "" {
		outcome = sourceCacheFallback
	}
	sc.setSpanOutcome(ctx, outcome, fallbackReason)
	return nil
}

// cloneSource clones the project from GitHub, returning the token the clone
// ended up using.
func (c *gitFetchProject) cloneSource(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, opts cloneOpts) (string, error) {
	start := time.Now()
	// Deferred so a failed clone is timed too, since those are the runs most
	// worth comparing against a cache hit.
	defer func() {
		setSourceCacheSpanDuration(ctx, sourceCacheCloneDurationAttribute, time.Since(start))
	}()
	token, err := c.fetchSource(ctx, logger, comm, conf, opts)
	return token, errors.Wrap(err, "running fetch command")
}

// saveSourceCache scrubs the cloned tree of anything task-specific, uploads it,
// and puts the task's own authenticated origin URL back for the rest of the run.
func (c *gitFetchProject) saveSourceCache(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig, sc *sourceCache, opts cloneOpts) (bool, error) {
	if err := c.runCommands(ctx, logger, conf, c.buildPreSaveCommand(opts)); err != nil {
		return false, errors.Wrap(err, "scrubbing source tree before saving")
	}
	produced, saveErr := sc.save(ctx, comm, logger)
	catcher := grip.NewBasicCatcher()
	catcher.Add(saveErr)
	catcher.Wrap(c.runCommands(ctx, logger, conf, c.buildPostSaveCommand(opts)), "restoring origin URL after saving")
	return produced, catcher.Resolve()
}

// buildPostRestoreCommand returns the script run over a restored tree. It verifies
// HEAD, restores this task's credentials, reconciles the checked out branch, and
// optionally checks out a PR.
func (c *gitFetchProject) buildPostRestoreCommand(conf *internal.TaskConfig, opts cloneOpts, revision string, runPRCheckout bool) []string {
	cmds := c.scriptInProjectDir(fmt.Sprintf(`test "$(git rev-parse HEAD)" = "%s"`, revision))
	cmds = append(cmds, setOriginURLCommands(opts)...)
	// The archive was scrubbed of the producer's credentials, so a restored
	// tree needs this task's own put into the submodule remotes.
	cmds = append(cmds, restoreGitConfigCredentialsCommands(opts.token)...)
	if runPRCheckout {
		cmds = append(cmds, prCheckoutCommands(conf)...)
	} else if opts.branch != "" {
		// The key pins the revision but not the branch, so a producer that cloned a
		// different branch at the same commit would leave its branch name behind.
		cmds = append(cmds, fmt.Sprintf(`git checkout -B '%s' %s`, opts.branch, revision))
	}
	return append(cmds, "git log --oneline -n 10")
}

// buildPreSaveCommand returns the script run before archiving the cloned tree.
// Hooks in a restored .git execute on checkout, and origin holds the producer's
// own credential, so both are scrubbed.
func (c *gitFetchProject) buildPreSaveCommand(opts cloneOpts) []string {
	tokenless := cloneOpts{owner: opts.owner, repo: opts.repo}
	cmds := append(c.scriptInProjectDir("rm -rf .git/hooks"), setOriginURLCommands(tokenless)...)
	return append(cmds, scrubGitConfigCredentialsCommand)
}

// rewriteGitConfigsCommand applies a sed expression to every config file under .git.
// Submodule URLs carry the parent's credential, so touching origin alone would leave
// the producer's token in the archive.
func rewriteGitConfigsCommand(sedExpr string) string {
	return fmt.Sprintf(`find .git -type f -name config | while read -r cfg; do sed -E '%s' "$cfg" > "$cfg.tmp" && mv "$cfg.tmp" "$cfg"; done`, sedExpr)
}

// scrubGitConfigCredentialsCommand strips the userinfo from every remote URL recorded under .git.
var scrubGitConfigCredentialsCommand = rewriteGitConfigsCommand(`s#://[^/@]*@#://#g`)

// restoreGitConfigCredentialsCommands is the inverse of
// scrubGitConfigCredentialsCommand for submodules: it puts the task's own
// credential back into the scrubbed GitHub URLs so submodule fetches keep
// working. URLs that already carry userinfo, such as the origin the caller set
// separately, do not match and are left alone.
func restoreGitConfigCredentialsCommands(token string) []string {
	if token == "" {
		return nil
	}
	return []string{
		"set +o xtrace",
		fmt.Sprintf("echo %s", strconv.Quote("restoring submodule credentials in .git configs")),
		rewriteGitConfigsCommand(fmt.Sprintf(restoreSubmoduleCredentialsRewrite, token)),
		"set -o xtrace",
	}
}

// buildPostSaveCommand puts the task's own credentials back after the scrubbed
// tree has been archived, for origin and for any submodule remotes the scrub
// stripped.
func (c *gitFetchProject) buildPostSaveCommand(opts cloneOpts) []string {
	cmds := append(c.scriptInProjectDir(), setOriginURLCommands(opts)...)
	return append(cmds, restoreGitConfigCredentialsCommands(opts.token)...)
}

// scriptInProjectDir starts a git script in the project directory, with cmds
// appended after the shell options every one of these scripts sets.
func (c *gitFetchProject) scriptInProjectDir(cmds ...string) []string {
	return append([]string{
		"set -o xtrace",
		"set -o errexit",
		fmt.Sprintf("cd %s", c.Directory),
	}, cmds...)
}

// setOriginURLCommands points origin at the given URL with xtrace off, since the
// URL embeds a token when one is set.
func setOriginURLCommands(opts cloneOpts) []string {
	return []string{
		"set +o xtrace",
		fmt.Sprintf("echo %s", strconv.Quote("git remote set-url origin <redacted>")),
		fmt.Sprintf("git remote set-url origin %s", thirdparty.FormGitURLForApp(opts.owner, opts.repo, opts.token)),
		"set -o xtrace",
	}
}

func (c *gitFetchProject) runCommands(ctx context.Context, logger client.LoggerProducer, conf *internal.TaskConfig, cmds []string) error {
	script := strings.Join(cmds, "\n")
	logger.Task().Debugf(ctx, "Commands are: %s", script)
	return c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", script}).Directory(conf.WorkDir).
		SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Execution().GetSender()).Run(ctx)
}

// fetchSource clones the project, returning the token it ended up using. A
// retry can refresh the token, and callers that rewrite the origin URL
// afterwards need the current one rather than the one they passed in.
func (c *gitFetchProject) fetchSource(ctx context.Context, logger client.LoggerProducer, comm client.Communicator, conf *internal.TaskConfig, opts cloneOpts) (string, error) {
	attempt := 0
	token := opts.token
	err := c.retryFetch(ctx, logger, comm, conf, true, opts, func(opts cloneOpts) error {
		attempt++
		// Don't refresh c.Token because it is user supplied.
		if attempt == gitFetchProjectRetries && c.Token == "" {
			refreshed, err := refreshCloneToken(ctx, comm, conf, opts.owner, opts.repo)
			if err != nil {
				logger.Task().Warningf(ctx, "Refreshing clone token, retrying with the previous one: %s", err)
			} else {
				token = refreshed
			}
		}
		opts.token = token

		// On the second attempt, check if the merge queue ref was deleted before retrying.
		if attempt == 2 {
			deleted, checkErr := mergeQueueRefDeleted(ctx, comm, conf, c.Token, opts)
			if checkErr != nil {
				return checkErr
			}
			if deleted {
				c.refNotFound = true
				return errors.New(mergeQueueRefGoneMessage)
			}
		}

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
		fetchSourceCmd := c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", fetchScript}).Directory(conf.WorkDir).
			SetOutputSender(level.Info, logger.Task().GetSender()).SetErrorSender(level.Error, logger.Execution().GetSender())

		logger.Task().Info(ctx, "Fetching source from git...")
		logger.Task().Debugf(ctx, "Commands are: %s", fetchScript)

		ctx, span := getTracer().Start(ctx, "clone_source", trace.WithAttributes(
			attribute.String(cloneOwnerAttribute, opts.owner),
			attribute.String(cloneRepoAttribute, opts.repo),
			attribute.String(cloneBranchAttribute, opts.branch),
			attribute.Int(cloneAttemptAttribute, attempt),
		))
		defer span.End()

		if err = fetchSourceCmd.Run(ctx); err != nil {
			span.SetAttributes(attribute.String(cloneErrorAttribute, err.Error()))
			return err
		}

		return nil
	})
	return token, err
}

// mergeQueueRefGoneMessage is the error a merge queue task fails with once its head ref is confirmed deleted.
const mergeQueueRefGoneMessage = "the GitHub merge SHA is not available most likely because the merge completed or was aborted"

// mergeQueueRefExists is a variable so tests can check the fail-fast path without reaching GitHub.
var mergeQueueRefExists = thirdparty.MergeQueueRefExists

// mergeQueueRefDeleted reports whether this merge queue task's head ref is gone from GitHub, meaning the
// queue item merged or was kicked out. It reports false whenever there is no definite answer to be had.
func mergeQueueRefDeleted(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, userToken string, opts cloneOpts) (bool, error) {
	if conf.Task.Requester != evergreen.GithubMergeRequester || conf.GithubMergeData.HeadBranch == "" {
		return false, nil
	}

	// A user-supplied clone token can't authenticate an app-scoped API call.
	appToken := opts.token
	if userToken != "" {
		var err error
		appToken, err = comm.CreateInstallationTokenForClone(ctx, conf.TaskData(), opts.owner, opts.repo, false)
		if err != nil {
			return false, nil
		}
	}
	if appToken == "" {
		return false, nil
	}

	exists, err := mergeQueueRefExists(ctx, opts.owner, opts.repo, "heads/"+conf.GithubMergeData.HeadBranch, appToken)
	if err != nil {
		return false, errors.Wrap(err, "checking if merge queue ref exists")
	}
	return !exists, nil
}

func refreshCloneToken(ctx context.Context, comm client.Communicator, conf *internal.TaskConfig, owner, repo string) (string, error) {
	token, err := comm.CreateInstallationTokenForClone(ctx, conf.TaskData(), owner, repo, true)
	if err != nil {
		return "", errors.Wrap(err, "creating app token")
	}
	conf.NewExpansions.Redact(generatedTokenKey, token)
	return token, nil
}

func (c *gitFetchProject) retryFetch(ctx context.Context, logger client.LoggerProducer, comm client.Communicator, conf *internal.TaskConfig, isSource bool, opts cloneOpts, fetch func(cloneOpts) error) error {
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
			if isSource {
				c.refNotFound = false
			}
			if attemptNum > 2 {
				opts.useVerbose = true // use verbose for the last 2 attempts
				logger.Task().Error(ctx, message.Fields{
					"message":      fmt.Sprintf("running git '%s' clone with verbose output", fetchType),
					"num_attempts": gitFetchProjectRetries,
					"attempt":      attemptNum,
				})
			}
			if err := fetch(opts); err != nil {
				attemptNum++
				if isSource && attemptNum == 1 {
					logger.Task().Warning(ctx, "git source clone failed with cached merge SHA; re-requesting merge SHA from GitHub")
				}
				if isSource && c.refNotFound {
					if markErr := comm.MarkMergeQueueGitRefNotFound(ctx, conf.TaskData()); markErr != nil {
						logger.Task().Warningf(ctx, "Failed to mark git ref not found: %s", markErr)
					}
					return false, errors.Wrap(err, mergeQueueRefGoneMessage)
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
	p *patch.Patch,
	moduleName string) error {

	var err error
	logger.Task().Infof(ctx, "Fetching module '%s'.", moduleName)

	var module *model.Module
	module, err = conf.Project.GetModuleByName(moduleName)
	if err != nil {
		return errors.Wrapf(err, "getting module '%s'", moduleName)
	}
	if module == nil {
		return errors.Errorf("module '%s' not found", moduleName)
	}

	moduleBase := filepath.ToSlash(filepath.Join(conf.ModulePaths[module.Name], module.Name))

	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return errors.Wrapf(err, "getting owner and repo for '%s'", moduleName)
	}

	var revision string

	// GitHub wikis are always cloned at remote HEAD; ref, set-module, manifest, and command params are ignored.
	if !model.IsWikiRepo(repo) {
		revision, err = c.getModuleRevision(ctx, conf, logger, p, module)
		if err != nil {
			return errors.Wrapf(err, "getting module revision for '%s'", moduleName)
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
	opts.owner = owner
	opts.repo = repo
	if strings.Contains(module.Repo, "git@github.com:") {
		logger.Task().Warningf(ctx, "ssh cloning is being deprecated. We are manually converting '%s'"+
			" to https format. Please update your project config.", module.Repo)
	}

	tokenRepo := parentRepoForGitHubAppToken(repo)
	appToken, err := comm.CreateInstallationTokenForClone(ctx, conf.TaskData(), owner, tokenRepo, false)
	if err != nil {
		return errors.Wrap(err, "creating app token")
	}

	opts.token = appToken

	// After generating, redact the token from the logs.
	conf.NewExpansions.Redact(generatedTokenKey, appToken)

	if err = opts.validate(); err != nil {
		return errors.Wrap(err, "validating clone options")
	}

	var modulePatch *patch.ModulePatch
	if p != nil {
		modulePatch = p.FindModule(moduleName)
	}

	attempt := 0
	token := appToken
	return c.retryFetch(ctx, logger, comm, conf, false, opts, func(opts cloneOpts) error {
		attempt++
		if attempt == gitFetchProjectRetries {
			refreshed, err := refreshCloneToken(ctx, comm, conf, owner, tokenRepo)
			if err != nil {
				logger.Task().Warningf(ctx, "Refreshing clone token, retrying with the previous one: %s", err)
			} else {
				token = refreshed
			}
		}
		opts.token = token

		moduleCmds, err := c.buildModuleCloneCommand(conf, opts, revision, modulePatch)
		if err != nil {
			return err
		}

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
		err = c.JasperManager().CreateCommand(ctx).Add([]string{"bash", "-c", strings.Join(moduleCmds, "\n")}).
			Directory(filepath.ToSlash(GetWorkingDirectory(conf, c.Directory))).
			SetOutputWriter(stdOut).SetErrorWriter(stdErr).Run(ctx)

		// Prefix every line of the output with the module name.
		output := strings.ReplaceAll(stdOut.String(), "\n", fmt.Sprintf("\n%s: ", module.Name))
		logger.Task().Info(ctx, output)

		errOutput := stdErr.String()
		if errOutput != "" {
			errOutput = strings.ReplaceAll(errOutput, "\n", fmt.Sprintf("\n%s: ", module.Name))
			logger.Task().Error(ctx, fmt.Sprintf("%s: %s", module.Name, errOutput))
		}

		if err != nil {
			span.SetAttributes(attribute.String(cloneErrorAttribute, err.Error()))
		}

		return err
	})
}

// getModuleRevision returns the git ref (commit, branch name, etc.) to use for a module.
// Sources are checked in order; the first non-empty applicable value wins:
//
//  1. For GitHub merge queue tasks, use the module.Branch (aka merge queue group's HEAD).
//  2. The ref specified by set-module
//  3. The ref specified by the parameter to git.get_project
//  4. The revision specified by the manifest expansion
//  5. module.Ref from project config.
//  6. module.Branch from project config.
//
// Wiki modules should not call this function as they do not checkout a specific revision.
func (c *gitFetchProject) getModuleRevision(ctx context.Context, conf *internal.TaskConfig, logger client.LoggerProducer, p *patch.Patch, module *model.Module) (string, error) {
	if p != nil {
		modulePatch := p.FindModule(module.Name)
		if modulePatch != nil {
			if conf.Task.Requester == evergreen.GithubMergeRequester {
				c.logModuleRevision(ctx, logger, module.Branch, module.Name, "defaulting to HEAD for merge")
				return module.Branch, nil
			} else {
				if revision := modulePatch.Githash; revision != "" {
					c.logModuleRevision(ctx, logger, revision, module.Name, "specified in set-module")
					return revision, nil
				}
			}
		}
	}

	if revision := c.Revisions[module.Name]; revision != "" {
		c.logModuleRevision(ctx, logger, revision, module.Name, "specified as parameter to git.get_project")
		return revision, nil
	}

	if revision := conf.Expansions.Get(moduleRevExpansionName(module.Name)); revision != "" {
		c.logModuleRevision(ctx, logger, revision, module.Name, "from manifest")
		return revision, nil
	}

	if revision := module.Ref; revision != "" {
		c.logModuleRevision(ctx, logger, revision, module.Name, "ref field in config file")
		return revision, nil
	}

	// Always fallback to the module's branch.
	c.logModuleRevision(ctx, logger, module.Branch, module.Name, "branch field in config file")
	return module.Branch, nil
}

func (c *gitFetchProject) fetch(ctx context.Context,
	comm client.Communicator,
	logger client.LoggerProducer,
	conf *internal.TaskConfig,
	opts cloneOpts) error {

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Clone the project, or restore it from the source cache.
	if err := c.fetchOrRestoreSource(ctx, comm, logger, conf, opts); err != nil {
		return err
	}

	// Retrieve the patch for the version if one exists.
	var p *patch.Patch
	var err error
	if evergreen.IsPatchRequester(conf.Task.Requester) {
		logger.Task().Info(ctx, "Fetching patch.")
		p, err = comm.GetTaskPatch(ctx, conf.TaskData())
		if err != nil {
			return errors.Wrap(err, "getting patch for task")
		}
	}

	// For every module, expand the module prefix.
	for _, moduleName := range conf.BuildVariant.Modules {
		module, err := conf.Project.GetModuleByName(moduleName)
		if err != nil {
			return errors.Wrapf(err, "getting module '%s'", moduleName)
		}
		if module == nil {
			return errors.Errorf("module '%s' not found", moduleName)
		}
		expandModulePrefix(ctx, conf, module.Name, module.Prefix, logger)
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	// Clone the project's modules in goroutines.
	for _, moduleName := range conf.BuildVariant.Modules {
		g.Go(func() error {
			if err := gCtx.Err(); err != nil {
				return nil
			}
			return errors.Wrapf(c.fetchModuleSource(gCtx, comm, conf, logger, p, moduleName), "fetching module source '%s'", moduleName)
		})
	}

	if err = g.Wait(); err != nil {
		return errors.Wrap(err, "fetching project and module source")
	}

	// Apply patches if this is a patch and we haven't already gotten the changes from a PR.
	if evergreen.IsPatchRequester(conf.Task.Requester) && !shouldSkipApplyingPatches(conf) {
		if err = c.getPatchContents(ctx, comm, logger, conf, p); err != nil {
			err = errors.Wrap(err, "getting patch contents")
			logger.Task().Error(ctx, err.Error())
			return err
		}

		// in order for the main commit's manifest to include module changes commit queue
		// commits need to be in the correct order, first modules and then the main patch
		// reorder patches so the main patch gets applied last
		if err = c.applyPatch(ctx, logger, conf, reorderPatches(p.Patches)); err != nil {
			err = errors.Wrap(err, "applying patch")
			logger.Task().Error(ctx, err.Error())
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

func (c *gitFetchProject) logModuleRevision(ctx context.Context, logger client.LoggerProducer, revision, module, reason string) {
	logger.Execution().Infof(ctx, "Using revision/ref '%s' for module '%s' (reason: %s).", revision, module, reason)
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

	for i, patchPart := range patch.Patches {
		// If the patch isn't stored externally, no need to do anything.
		if patchPart.PatchSet.PatchFileId == "" {
			continue
		}

		if err := ctx.Err(); err != nil {
			return errors.Wrapf(err, "canceled while getting patch file '%s'", patchPart.ModuleName)
		}

		// otherwise, fetch the contents and load it into the patch object
		logger.Execution().Infof(ctx, "Fetching patch contents for patch file '%s'.", patchPart.PatchSet.PatchFileId)

		result, err := comm.GetPatchFile(ctx, conf.TaskData(), patchPart.PatchSet.PatchFileId)
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
				logger.Execution().Infof(ctx,
					"Skipping patch for module '%s': the current build variant does not use it.",
					module.Name)
				continue
			}
			expandModulePrefix(ctx, conf, module.Name, module.Prefix, logger)
			moduleDir = filepath.ToSlash(filepath.Join(conf.ModulePaths[module.Name], module.Name))
		}

		if len(patchPart.PatchSet.Patch) == 0 {
			logger.Execution().Info(ctx, "Skipping empty patch file...")
			continue

		} else if patchPart.ModuleName == "" {
			logger.Execution().Info(ctx, "Applying patch with git...")

		} else {
			logger.Execution().Infof(ctx, "Applying '%s' module patch with git...", patchPart.ModuleName)
		}

		// create a temporary folder and store patch files on disk,
		// for later use in shell script
		tempFile, err := os.CreateTemp("", "mcipatch_")
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() { //nolint:evg-lint
			grip.Error(ctx, tempFile.Close())
			grip.Error(ctx, os.Remove(tempFile.Name()))
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
			if patchPart.ModuleName == "" && c.Filter != "" && len(c.SparseCheckoutPaths) > 0 {
				return errors.Wrap(err, "applying patch in a sparse checkout (a patch that touches files outside sparse_checkout_paths cannot be applied)")
			}
			return errors.WithStack(err)
		}
	}
	return nil
}

// usesGitHubParentPRCheckout reports whether the main project checkout should use
// a GitHub PR or merge-queue ref instead of the task revision.
func usesGitHubParentPRCheckout(conf *internal.TaskConfig) bool {
	if conf.GitHubParentPRCheckout != nil && conf.GitHubParentPRCheckout.ForSource {
		return true
	}
	return conf.Task.ParentPatchID == "" &&
		(conf.GithubPatchData.PRNumber != 0 || conf.GithubMergeData.HeadSHA != "")
}

// shouldSkipApplyingPatches reports whether git apply should be skipped for this patch task.
func shouldSkipApplyingPatches(conf *internal.TaskConfig) bool {
	return usesGitHubParentPRCheckout(conf)
}

// moduleUsesGitHubParentPRCheckout reports whether a module clone should use a
// GitHub PR checkout instead of the module revision.
func moduleUsesGitHubParentPRCheckout(conf *internal.TaskConfig, modulePatch *patch.ModulePatch) bool {
	if modulePatch == nil || conf.GitHubParentPRCheckout == nil {
		return false
	}
	return conf.GitHubParentPRCheckout.ForModule == modulePatch.ModuleName &&
		conf.GitHubParentPRCheckout.PRNumber != 0 &&
		conf.GitHubParentPRCheckout.HeadHash != ""
}

// parentRepoForGitHubAppToken returns the repository name to pass when resolving
// a GitHub App installation for clone tokens. Installations are on the parent
// repository; clone URLs may still use the ".wiki" repository name.
func parentRepoForGitHubAppToken(repo string) string {
	if !model.IsWikiRepo(repo) {
		return repo
	}
	r := strings.TrimSuffix(strings.TrimSpace(repo), ".git")
	return strings.TrimSuffix(r, ".wiki")
}
