package model

import (
	"context"
	"strings"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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

// committedConfigRequesters are the requesters whose versions' config is
// committed to the tracked branch. Git tag versions are excluded because a tag
// can point at a commit that was never merged.
var committedConfigRequesters = []string{
	evergreen.RepotrackerVersionRequester,
	evergreen.TriggerRequester,
	evergreen.AdHocRequester,
}

const committedManifestLookback = 10

// These are variables so tests can check module repo trust without reaching
// GitHub.
var (
	isGitHubRepoPublic          = thirdparty.IsGitHubRepoPublic
	gitHubUserHasReadPermission = thirdparty.GitHubUserHasReadPermission
)

// ValidatePatchModuleRepos returns an error if the patch's modules use repos
// the patch isn't allowed to use. Otherwise, a patch author could read any repo
// Evergreen's GitHub App can by declaring it as a module.
func ValidatePatchModuleRepos(ctx context.Context, p *patch.Patch, projectRef *ProjectRef, modules ModuleList) error {
	return newPatchModuleRepoChecker(projectRef, p).validateModules(ctx, modules)
}

// TaskRepoAccess checks which repos a task can get clone tokens for.
type TaskRepoAccess struct {
	settings *evergreen.Settings
	task     *task.Task
	forUser  bool

	projectRef   *ProjectRef
	moduleTrust  map[string]manifest.ModuleRepoTrust
	patchChecker *patchModuleRepoChecker
}

// NewTaskRepoAccess returns a checker for the repos the task can clone. Set
// forUser for requests on behalf of a user, who may not be the PR author.
func NewTaskRepoAccess(ctx context.Context, settings *evergreen.Settings, t *task.Task, forUser bool) (*TaskRepoAccess, error) {
	projectRef, err := FindMergedProjectRef(ctx, t.Project, t.Version, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project '%s'", t.Project)
	}
	if projectRef == nil {
		return nil, errors.Errorf("project ref '%s' doesn't exist", t.Project)
	}
	return &TaskRepoAccess{
		settings:   settings,
		task:       t,
		forUser:    forUser,
		projectRef: projectRef,
	}, nil
}

// IsAllowed returns whether the task can get a token to clone owner/repo.
func (a *TaskRepoAccess) IsAllowed(ctx context.Context, owner, repo string) (bool, error) {
	if isSameRepo(a.projectRef.Owner, a.projectRef.Repo, owner, repo) {
		return true, nil
	}
	if err := a.loadModules(ctx); err != nil {
		return false, err
	}
	trust, ok := a.moduleTrust[moduleRepoKey(owner, repo)]
	if !ok {
		return false, nil
	}
	if !evergreen.IsPatchRequester(a.task.Requester) {
		return true, nil
	}
	if trust == "" {
		// The patch was finalized before trust was recorded.
		var err error
		trust, err = a.checkPatchModuleRepo(ctx, owner, repo)
		if err != nil {
			return false, err
		}
	}

	switch trust {
	case manifest.ModuleRepoTrustCommitted, manifest.ModuleRepoTrustPublic, manifest.ModuleRepoTrustReviewedConfig:
		return true, nil
	case manifest.ModuleRepoTrustPRAuthor:
		return !a.forUser, nil
	default:
		return false, nil
	}
}

func (a *TaskRepoAccess) checkPatchModuleRepo(ctx context.Context, owner, repo string) (manifest.ModuleRepoTrust, error) {
	if a.patchChecker == nil {
		p, err := patch.FindOneId(ctx, a.task.Version)
		if err != nil {
			return "", errors.Wrapf(err, "finding patch '%s'", a.task.Version)
		}
		if p == nil {
			return "", errors.Errorf("patch '%s' not found", a.task.Version)
		}
		a.patchChecker = newPatchModuleRepoChecker(a.projectRef, p)
	}
	return a.patchChecker.trust(ctx, owner, repo)
}

func (a *TaskRepoAccess) loadModules(ctx context.Context) error {
	if a.moduleTrust != nil {
		return nil
	}
	moduleTrust := map[string]manifest.ModuleRepoTrust{}

	mfest, err := manifest.FindOne(ctx, manifest.ById(a.task.Version))
	if err != nil {
		return errors.Wrap(err, "finding manifest for task")
	}
	if mfest != nil {
		for _, m := range mfest.Modules {
			if m != nil {
				moduleTrust[moduleRepoKey(m.Owner, m.Repo)] = m.RepoTrust
			}
		}
		a.moduleTrust = moduleTrust
		return nil
	}

	v, err := VersionFindOne(ctx, VersionById(a.task.Version))
	if err != nil {
		return errors.Wrap(err, "finding version for task")
	}
	if v == nil {
		return errors.Errorf("version '%s' not found", a.task.Version)
	}
	project, _, err := FindAndTranslateProjectForVersion(ctx, a.settings, v, false)
	if err != nil {
		return errors.Wrap(err, "loading project for version")
	}
	if project == nil {
		return errors.Errorf("project for version '%s' not found", a.task.Version)
	}
	for _, m := range project.Modules {
		moduleOwner, moduleRepo, err := m.GetOwnerAndRepo()
		if err != nil {
			continue
		}
		moduleTrust[moduleRepoKey(moduleOwner, moduleRepo)] = ""
	}
	a.moduleTrust = moduleTrust
	return nil
}

// GitHubAppTokenRepo returns the name of the repo to request a GitHub App
// token for. A GitHub wiki uses its parent repo's token.
func GitHubAppTokenRepo(repo string) string {
	r := strings.TrimSuffix(strings.TrimSpace(repo), ".git")
	return strings.TrimSuffix(r, ".wiki")
}

func moduleRepoKey(owner, repo string) string {
	return strings.ToLower(strings.TrimSpace(owner) + "/" + GitHubAppTokenRepo(repo))
}

func isSameRepo(owner1, repo1, owner2, repo2 string) bool {
	return moduleRepoKey(owner1, repo1) == moduleRepoKey(owner2, repo2)
}

// patchModuleRepoChecker checks whether a patch can use module repos. A nil
// checker allows every repo.
type patchModuleRepoChecker struct {
	projectRef     *ProjectRef
	patch          *patch.Patch
	reviewedConfig bool

	// The mutex is needed because includes are read concurrently.
	mu             sync.Mutex
	loaded         bool
	checksDisabled bool
	committed      map[string]bool
	results        map[string]manifest.ModuleRepoTrust
}

func newPatchModuleRepoChecker(projectRef *ProjectRef, p *patch.Patch) *patchModuleRepoChecker {
	if projectRef == nil || p == nil {
		return nil
	}
	return &patchModuleRepoChecker{
		projectRef: projectRef,
		patch:      p,
		// Not IsMergeQueuePatch, since CLI patches can use the merge queue alias.
		reviewedConfig: p.GithubMergeData.HeadSHA != "" || p.IsChild(),
		results:        map[string]manifest.ModuleRepoTrust{},
	}
}

func (c *patchModuleRepoChecker) validateModules(ctx context.Context, modules ModuleList) error {
	if c == nil || c.reviewedConfig || len(modules) == 0 {
		return nil
	}
	projVars, err := FindMergedProjectVars(ctx, c.projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "getting project vars for project '%s'", c.projectRef.Id)
	}

	var untrusted []string
	for _, module := range modules {
		if projVars != nil {
			if err := util.ExpandValues(&module, util.NewExpansions(projVars.Vars)); err != nil {
				return errors.Wrapf(err, "expanding module '%s'", module.Name)
			}
		}
		owner, repo, err := module.GetOwnerAndRepo()
		if err != nil {
			return errors.Wrapf(err, "getting owner and repo for module '%s'", module.Name)
		}
		trust, err := c.trust(ctx, owner, repo)
		if err != nil {
			return errors.Wrapf(err, "checking repo for module '%s'", module.Name)
		}
		if trust == manifest.ModuleRepoTrustUntrusted {
			untrusted = append(untrusted, module.Name)
		}
	}
	if len(untrusted) == 0 {
		return nil
	}
	return c.enforce(ctx, errors.Errorf("patches can't use modules '%s' because their repos are private and aren't modules in a recent mainline version", strings.Join(untrusted, "', '")))
}

func (c *patchModuleRepoChecker) check(ctx context.Context, owner, repo string) error {
	if c == nil {
		return nil
	}
	trust, err := c.trust(ctx, owner, repo)
	if err != nil {
		return errors.Wrapf(err, "checking module repo '%s/%s'", owner, repo)
	}
	if trust != manifest.ModuleRepoTrustUntrusted {
		return nil
	}
	return c.enforce(ctx, errors.Errorf("patches can't use repo '%s/%s' because it's private and isn't a module in a recent mainline version", owner, repo))
}

// recordTrust records why the patch can use each module repo so clones don't
// need to check again.
func (c *patchModuleRepoChecker) recordTrust(ctx context.Context, mfst *manifest.Manifest) error {
	if c == nil || mfst == nil {
		return nil
	}
	for name, mod := range mfst.Modules {
		if mod == nil {
			continue
		}
		trust, err := c.trust(ctx, mod.Owner, mod.Repo)
		if err != nil {
			return errors.Wrapf(err, "checking repo for module '%s'", name)
		}
		mod.RepoTrust = trust
	}
	return nil
}

func (c *patchModuleRepoChecker) enforce(ctx context.Context, err error) error {
	if !c.checksDisabled {
		return err
	}
	grip.Warning(ctx, message.WrapError(err, message.Fields{
		"message":  "allowing untrusted patch module repo because checks are disabled",
		"patch_id": c.patch.Id.Hex(),
		"project":  c.projectRef.Id,
	}))
	return nil
}

func (c *patchModuleRepoChecker) trust(ctx context.Context, owner, repo string) (manifest.ModuleRepoTrust, error) {
	if c.reviewedConfig {
		return manifest.ModuleRepoTrustReviewedConfig, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.load(ctx); err != nil {
		return "", err
	}
	key := moduleRepoKey(owner, repo)
	if c.committed[key] {
		return manifest.ModuleRepoTrustCommitted, nil
	}
	if trust, ok := c.results[key]; ok {
		return trust, nil
	}

	trust, err := c.trustOnGitHub(ctx, owner, GitHubAppTokenRepo(repo))
	if err != nil {
		return "", err
	}
	c.results[key] = trust
	grip.InfoWhen(ctx, trust == manifest.ModuleRepoTrustUntrusted, message.Fields{
		"message":   "patch module repo is not trusted",
		"patch_id":  c.patch.Id.Hex(),
		"project":   c.projectRef.Id,
		"owner":     owner,
		"repo":      repo,
		"author":    c.patch.Author,
		"pr_author": c.patch.GithubPatchData.Author,
	})
	return trust, nil
}

// trustOnGitHub checks whether the repo is public or readable by the PR author.
// CLI patches can't use the author's access because Evergreen users' GitHub
// usernames aren't verified.
func (c *patchModuleRepoChecker) trustOnGitHub(ctx context.Context, owner, repo string) (manifest.ModuleRepoTrust, error) {
	public, err := isGitHubRepoPublic(ctx, owner, repo)
	if err != nil {
		return "", errors.Wrap(err, "checking if repo is public")
	}
	if public {
		return manifest.ModuleRepoTrustPublic, nil
	}
	prAuthor := c.patch.GithubPatchData.Author
	if !c.patch.IsGithubPRPatch() || prAuthor == "" {
		return manifest.ModuleRepoTrustUntrusted, nil
	}
	canRead, err := gitHubUserHasReadPermission(ctx, owner, repo, prAuthor)
	if err != nil {
		// GitHub errors for users who aren't collaborators.
		grip.Warning(ctx, message.WrapError(err, message.Fields{
			"message":   "checking if PR author can read patch module repo",
			"patch_id":  c.patch.Id.Hex(),
			"owner":     owner,
			"repo":      repo,
			"pr_author": prAuthor,
		}))
		return manifest.ModuleRepoTrustUntrusted, nil
	}
	if canRead {
		return manifest.ModuleRepoTrustPRAuthor, nil
	}
	return manifest.ModuleRepoTrustUntrusted, nil
}

// load loads the repos declared in committed config. Recent mainline
// versions are needed as well as the base version, since a patch's base commit
// may have no version or predate a module.
func (c *patchModuleRepoChecker) load(ctx context.Context) error {
	if c.loaded {
		return nil
	}
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags")
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

	c.checksDisabled = flags.ModuleRepoChecksDisabled
	c.committed = committed
	c.loaded = true
	return nil
}

// findCommittedManifests returns the manifests of the mainline version at the
// revision and the project's recent committed-config versions.
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
		VersionRequesterKey:  bson.M{"$in": committedConfigRequesters},
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
