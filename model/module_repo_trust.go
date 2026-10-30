package model

import (
	"context"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// committedManifestLookback bounds how many recent mainline versions' manifests
// are checked for module repos the project already trusts.
const committedManifestLookback = 10

// These are variables so tests can check module repo trust without reaching
// GitHub.
var (
	isGitHubRepoPublic          = thirdparty.IsGitHubRepoPublic
	gitHubUserHasReadPermission = thirdparty.GitHubUserHasReadPermission
)

// ValidatePatchModuleRepos returns an error if any of the patch's modules use a
// repo the patch isn't allowed to use. Evergreen's GitHub App can read every
// repo it's installed on, so a patch author must not be able to reach a repo
// just by declaring it as a module. A patch can use the project's own repo,
// repos declared as modules in the project's mainline config, public repos,
// and, for GitHub PR patches, repos the PR author can read. Merge queue and
// child patches can use any repo because their authors can't change their
// config without review.
func ValidatePatchModuleRepos(ctx context.Context, p *patch.Patch, projectRef *ProjectRef, modules ModuleList) error {
	checker := newPatchModuleRepoChecker(projectRef, p)
	if checker == nil || len(modules) == 0 {
		return nil
	}
	projVars, err := FindMergedProjectVars(ctx, projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "getting project vars for project '%s'", projectRef.Id)
	}

	var untrusted []string
	for _, module := range modules {
		// Manifests resolve module commits using the expanded owner and repo.
		if projVars != nil {
			if err := util.ExpandValues(&module, util.NewExpansions(projVars.Vars)); err != nil {
				return errors.Wrapf(err, "expanding module '%s'", module.Name)
			}
		}
		owner, repo, err := module.GetOwnerAndRepo()
		if err != nil {
			return errors.Wrapf(err, "getting owner and repo for module '%s'", module.Name)
		}
		trusted, err := checker.isTrusted(ctx, owner, repo)
		if err != nil {
			return errors.Wrapf(err, "checking repo for module '%s'", module.Name)
		}
		if !trusted {
			untrusted = append(untrusted, module.Name)
		}
	}
	if len(untrusted) > 0 {
		return errors.Errorf("modules '%s' use repos that aren't declared in the project's mainline config; commit them to the tracked branch first (GitHub PR patches can also use repos the PR author can read)", strings.Join(untrusted, "', '"))
	}
	return nil
}

// IsRepoAllowedForTask returns whether the task can get a token to clone
// owner/repo. The repo must be the task's project repo or one of its version's
// modules. If the task is in a patch, the module repo must also pass the same
// checks as ValidatePatchModuleRepos.
func IsRepoAllowedForTask(ctx context.Context, settings *evergreen.Settings, t *task.Task, owner, repo string) (bool, error) {
	projectRef, err := FindMergedProjectRef(ctx, t.Project, t.Version, true)
	if err != nil {
		return false, errors.Wrapf(err, "finding project '%s'", t.Project)
	}
	if projectRef == nil {
		return false, errors.Errorf("project ref '%s' doesn't exist", t.Project)
	}

	if isSameRepo(projectRef.Owner, projectRef.Repo, owner, repo) {
		return true, nil
	}

	declared, err := isModuleRepoForVersion(ctx, settings, t, owner, repo)
	if err != nil {
		return false, err
	}
	if !declared || !evergreen.IsPatchRequester(t.Requester) {
		return declared, nil
	}

	p, err := patch.FindOneId(ctx, t.Version)
	if err != nil {
		return false, errors.Wrapf(err, "finding patch '%s'", t.Version)
	}
	if p == nil {
		return false, errors.Errorf("patch '%s' not found", t.Version)
	}
	return newPatchModuleRepoChecker(projectRef, p).isTrusted(ctx, owner, repo)
}

// isModuleRepoForVersion returns whether owner/repo is one of the modules in
// the task's version.
func isModuleRepoForVersion(ctx context.Context, settings *evergreen.Settings, t *task.Task, owner, repo string) (bool, error) {
	mfest, err := manifest.FindFromVersion(ctx, t.Version, t.Project, "", t.Requester)
	if err != nil {
		return false, errors.Wrap(err, "finding manifest for task")
	}
	if mfest != nil {
		for _, m := range mfest.Modules {
			if isSameRepo(m.Owner, m.Repo, owner, repo) {
				return true, nil
			}
		}
		return false, nil
	}

	v, err := VersionFindOne(ctx, VersionById(t.Version))
	if err != nil {
		return false, errors.Wrap(err, "finding version for task")
	}
	if v == nil {
		return false, errors.Errorf("version '%s' not found", t.Version)
	}

	project, _, err := FindAndTranslateProjectForVersion(ctx, settings, v, false)
	if err != nil {
		return false, errors.Wrap(err, "loading project for version")
	}
	if project == nil {
		return false, errors.Errorf("project for version '%s' not found", t.Version)
	}

	for _, m := range project.Modules {
		moduleOwner, moduleRepo, err := m.GetOwnerAndRepo()
		if err != nil {
			continue
		}
		if isSameRepo(moduleOwner, moduleRepo, owner, repo) {
			return true, nil
		}
	}
	return false, nil
}

// GitHubAppTokenRepo returns the name of the repo to request a GitHub App
// token for. A GitHub wiki uses its parent repo's token.
func GitHubAppTokenRepo(repo string) string {
	r := strings.TrimSuffix(strings.TrimSpace(repo), ".git")
	return strings.TrimSuffix(r, ".wiki")
}

// moduleRepoKey identifies the repo whose token is needed to clone owner/repo.
func moduleRepoKey(owner, repo string) string {
	return strings.ToLower(strings.TrimSpace(owner) + "/" + GitHubAppTokenRepo(repo))
}

func isSameRepo(owner1, repo1, owner2, repo2 string) bool {
	return moduleRepoKey(owner1, repo1) == moduleRepoKey(owner2, repo2)
}

// patchModuleRepoChecker checks whether a patch is allowed to use module
// repos. It's safe for concurrent use.
type patchModuleRepoChecker struct {
	projectRef *ProjectRef
	patch      *patch.Patch

	mu sync.Mutex
	// committed holds the repos declared by the project's committed config.
	// It's nil until loaded.
	committed map[string]bool
	// results caches the result for repos outside of committed, which need
	// GitHub to check.
	results map[string]bool
}

// newPatchModuleRepoChecker returns a checker for the patch's module repos, or
// nil if the patch's config isn't controlled by its author. A nil checker
// trusts every repo.
func newPatchModuleRepoChecker(projectRef *ProjectRef, p *patch.Patch) *patchModuleRepoChecker {
	if projectRef == nil || p == nil || p.IsMergeQueuePatch() || p.IsChild() {
		return nil
	}
	return &patchModuleRepoChecker{
		projectRef: projectRef,
		patch:      p,
		results:    map[string]bool{},
	}
}

// check returns an error if the patch isn't allowed to use owner/repo.
func (c *patchModuleRepoChecker) check(ctx context.Context, owner, repo string) error {
	trusted, err := c.isTrusted(ctx, owner, repo)
	if err != nil {
		return errors.Wrapf(err, "checking module repo '%s/%s'", owner, repo)
	}
	if !trusted {
		return errors.Errorf("module repo '%s/%s' isn't declared in the project's mainline config; commit it to the tracked branch first (GitHub PR patches can also use repos the PR author can read)", owner, repo)
	}
	return nil
}

func (c *patchModuleRepoChecker) isTrusted(ctx context.Context, owner, repo string) (bool, error) {
	if c == nil {
		return true, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.loadCommitted(ctx); err != nil {
		return false, err
	}
	key := moduleRepoKey(owner, repo)
	if c.committed[key] {
		return true, nil
	}
	if trusted, ok := c.results[key]; ok {
		return trusted, nil
	}

	trusted, err := c.isTrustedOnGitHub(ctx, owner, GitHubAppTokenRepo(repo))
	if err != nil {
		return false, err
	}
	c.results[key] = trusted
	grip.InfoWhen(ctx, !trusted, message.Fields{
		"message":   "patch module repo is not trusted",
		"patch_id":  c.patch.Id.Hex(),
		"project":   c.projectRef.Id,
		"owner":     owner,
		"repo":      repo,
		"author":    c.patch.Author,
		"pr_author": c.patch.GithubPatchData.Author,
	})
	return trusted, nil
}

// isTrustedOnGitHub returns whether the repo is public or, for GitHub PR
// patches, readable by the PR author. The PR author comes from GitHub's
// webhook, whereas an Evergreen user's GitHub username isn't verified, so CLI
// patches can't rely on it.
func (c *patchModuleRepoChecker) isTrustedOnGitHub(ctx context.Context, owner, repo string) (bool, error) {
	public, err := isGitHubRepoPublic(ctx, owner, repo)
	if errors.Is(err, githubapp.ErrGitHubAppNotInstalled) {
		// Evergreen's GitHub App can't read the repo, so using it as a
		// module can't expose anything the patch author couldn't already
		// reach.
		return true, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "checking if repo is public")
	}
	if public {
		return true, nil
	}
	prAuthor := c.patch.GithubPatchData.Author
	if !c.patch.IsGithubPRPatch() || prAuthor == "" {
		return false, nil
	}
	canRead, err := gitHubUserHasReadPermission(ctx, owner, repo, prAuthor)
	if err != nil {
		// GitHub errors for users who aren't collaborators, so this can't
		// distinguish a failed lookup from a lack of access.
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message":   "checking if PR author can read patch module repo",
			"patch_id":  c.patch.Id.Hex(),
			"owner":     owner,
			"repo":      repo,
			"pr_author": prAuthor,
		}))
		return false, nil
	}
	return canRead, nil
}

// loadCommitted loads the project's repo and the module repos declared in the
// manifests of the patch's base version and the project's recent mainline
// versions. Both are needed because a patch can be based on a commit with no
// mainline version, or on an old commit from before a module was added.
func (c *patchModuleRepoChecker) loadCommitted(ctx context.Context) error {
	if c.committed != nil {
		return nil
	}
	committed := map[string]bool{
		moduleRepoKey(c.projectRef.Owner, c.projectRef.Repo): true,
	}

	manifests, err := findCommittedManifests(ctx, c.projectRef.Id, c.patch.Githash)
	if err != nil {
		return errors.Wrap(err, "finding mainline manifests")
	}
	for _, m := range manifests {
		for _, mod := range m.Modules {
			if mod != nil {
				committed[moduleRepoKey(mod.Owner, mod.Repo)] = true
			}
		}
	}

	c.committed = committed
	return nil
}

// findCommittedManifests returns the manifest of the mainline version at the
// revision, along with the manifests of the project's most recent versions
// whose config is committed to the tracked branch.
func findCommittedManifests(ctx context.Context, projectID, revision string) ([]manifest.Manifest, error) {
	var manifests []manifest.Manifest
	if revision != "" {
		base, err := manifest.FindOne(ctx, manifest.ByBaseProjectAndRevision(projectID, revision))
		if err != nil {
			return nil, errors.Wrapf(err, "finding manifest for base revision '%s'", revision)
		}
		if base != nil {
			manifests = append(manifests, *base)
		}
	}

	versions, err := VersionFind(ctx, db.Query(bson.M{
		VersionIdentifierKey: projectID,
		VersionRequesterKey:  bson.M{"$in": evergreen.SystemVersionRequesterTypes},
	}).WithFields(VersionIdKey).Sort([]string{"-" + VersionRevisionOrderNumberKey}).Limit(committedManifestLookback))
	if err != nil {
		return nil, errors.Wrap(err, "finding recent mainline versions")
	}
	if len(versions) == 0 {
		return manifests, nil
	}
	ids := make([]string, 0, len(versions))
	for _, v := range versions {
		ids = append(ids, v.Id)
	}
	recent, err := manifest.Find(ctx, manifest.ByIds(ids))
	if err != nil {
		return nil, errors.Wrap(err, "finding recent mainline manifests")
	}
	return append(manifests, recent...), nil
}
