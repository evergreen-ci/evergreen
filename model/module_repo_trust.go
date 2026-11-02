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
// committed to the tracked branch.
var committedConfigRequesters = []string{
	evergreen.RepotrackerVersionRequester,
	evergreen.TriggerRequester,
	evergreen.AdHocRequester,
}

const committedManifestLookback = 10

var (
	isGitHubRepoPublic          = thirdparty.IsGitHubRepoPublic
	gitHubUserHasReadPermission = thirdparty.GitHubUserHasReadPermission
)

// ValidatePatchModuleRepos returns an error if the patch's modules use repos
// the patch isn't allowed to use.
func ValidatePatchModuleRepos(ctx context.Context, p *patch.Patch, projectRef *ProjectRef, modules ModuleList) error {
	c := newPatchModuleRepoChecker(projectRef, p)
	if c == nil || len(modules) == 0 {
		return nil
	}
	projVars, err := FindMergedProjectVars(ctx, projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "getting project vars for project '%s'", projectRef.Id)
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
		allowed, err := c.Allows(ctx, owner, repo)
		if err != nil {
			return errors.Wrapf(err, "checking repo for module '%s'", module.Name)
		}
		if !allowed {
			untrusted = append(untrusted, module.Name)
		}
	}
	if len(untrusted) > 0 {
		return errors.Errorf("modules '%s' use private repos that patches can't use until a mainline version declares them", strings.Join(untrusted, "', '"))
	}
	return nil
}

// ModuleRepoCheckerForUser returns a checker for a user acting on the task, who
// may not be the PR author.
func ModuleRepoCheckerForUser(ctx context.Context, t *task.Task) (*PatchModuleRepoChecker, error) {
	if !evergreen.IsPatchRequester(t.Requester) {
		return nil, nil
	}
	p, err := patch.FindOneId(ctx, t.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "finding patch '%s'", t.Version)
	}
	if p == nil {
		return nil, errors.Errorf("patch '%s' not found", t.Version)
	}
	projectRef, err := FindMergedProjectRef(ctx, t.Project, t.Version, true)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project '%s'", t.Project)
	}
	if projectRef == nil {
		return nil, errors.Errorf("project ref '%s' doesn't exist", t.Project)
	}
	c := newPatchModuleRepoChecker(projectRef, p)
	if c != nil {
		c.allowPRAuthor = false
	}
	return c, nil
}

// PatchModuleRepoChecker checks whether a patch can use module repos.
type PatchModuleRepoChecker struct {
	projectRef    *ProjectRef
	patch         *patch.Patch
	allowPRAuthor bool
	mu            sync.Mutex
	committed     map[string]bool
	results       map[string]bool
}

func newPatchModuleRepoChecker(projectRef *ProjectRef, p *patch.Patch) *PatchModuleRepoChecker {
	if projectRef == nil || p == nil || p.GithubMergeData.HeadSHA != "" || p.IsChild() {
		return nil
	}
	return &PatchModuleRepoChecker{
		projectRef:    projectRef,
		patch:         p,
		allowPRAuthor: true,
		results:       map[string]bool{},
	}
}

// Allows returns whether the patch can use owner/repo.
func (c *PatchModuleRepoChecker) Allows(ctx context.Context, owner, repo string) (bool, error) {
	if c == nil {
		return true, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.load(ctx); err != nil {
		return false, err
	}
	key := moduleRepoKey(owner, repo)
	if c.committed[key] {
		return true, nil
	}
	trusted, ok := c.results[key]
	if !ok {
		var err error
		trusted, err = c.isTrustedOnGitHub(ctx, owner, strings.TrimSuffix(trimGitSuffix(repo), ".wiki"))
		if err != nil {
			return false, err
		}
		c.results[key] = trusted
	}
	return trusted, nil
}

func (c *PatchModuleRepoChecker) check(ctx context.Context, owner, repo string) error {
	allowed, err := c.Allows(ctx, owner, repo)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.Errorf("repo '%s/%s' is private, so patches can't use it until a mainline version declares it", owner, repo)
	}
	return nil
}

// isTrustedOnGitHub checks whether the repo is public or readable by the PR
// author.
func (c *PatchModuleRepoChecker) isTrustedOnGitHub(ctx context.Context, owner, repo string) (bool, error) {
	public, err := isGitHubRepoPublic(ctx, owner, repo)
	if err != nil {
		return false, errors.Wrap(err, "checking if repo is public")
	}
	prAuthor := c.patch.GithubPatchData.Author
	if public || !c.allowPRAuthor || !c.patch.IsGithubPRPatch() || prAuthor == "" {
		return public, nil
	}
	canRead, err := gitHubUserHasReadPermission(ctx, owner, repo, prAuthor)
	if err != nil {
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

func (c *PatchModuleRepoChecker) load(ctx context.Context) error {
	if c.committed != nil {
		return nil
	}

	var manifests []manifest.Manifest
	base, err := manifest.FindOne(ctx, manifest.ByBaseProjectAndRevision(c.projectRef.Id, c.patch.Githash))
	if err != nil {
		return errors.Wrapf(err, "finding manifest for base revision '%s'", c.patch.Githash)
	}
	if base != nil {
		manifests = append(manifests, *base)
	}
	versions, err := VersionFind(ctx, db.Query(bson.M{
		VersionIdentifierKey: c.projectRef.Id,
		VersionRequesterKey:  bson.M{"$in": committedConfigRequesters},
	}).WithFields(VersionIdKey).Sort([]string{"-" + VersionRevisionOrderNumberKey}).Limit(committedManifestLookback))
	if err != nil {
		return errors.Wrap(err, "finding recent mainline versions")
	}
	if len(versions) > 0 {
		ids := make([]string, 0, len(versions))
		for _, v := range versions {
			ids = append(ids, v.Id)
		}
		recent, err := manifest.Find(ctx, manifest.ByIds(ids))
		if err != nil {
			return errors.Wrap(err, "finding recent mainline manifests")
		}
		manifests = append(manifests, recent...)
	}

	c.committed = map[string]bool{moduleRepoKey(c.projectRef.Owner, c.projectRef.Repo): true}
	for _, m := range manifests {
		for _, mod := range m.Modules {
			if mod != nil {
				c.committed[moduleRepoKey(mod.Owner, mod.Repo)] = true
			}
		}
	}
	return nil
}

func trimGitSuffix(repo string) string {
	return strings.TrimSuffix(strings.TrimSpace(repo), ".git")
}

func moduleRepoKey(owner, repo string) string {
	return strings.ToLower(strings.TrimSpace(owner) + "/" + strings.TrimSuffix(trimGitSuffix(repo), ".wiki"))
}
