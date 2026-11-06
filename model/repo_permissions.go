package model

import (
	"context"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// RepoPermissionScope selects a maintained scope for a repo-level grant.
type RepoPermissionScope string

const (
	RepoOnlyScope         RepoPermissionScope = "repo_only"
	RepoAndProjectsScope  RepoPermissionScope = "repo_and_projects"
	RepoUnrestrictedScope RepoPermissionScope = "unrestricted_branches"
)

const (
	repoOnlyScopePrefix         = "repo_only_"
	repoAdminScopePrefix        = "admin_repo_"
	repoUnrestrictedScopePrefix = "unrestricted_branches_"
)

// RepoPermissionScopes returns the scope IDs used for grants on this repo.
func RepoPermissionScopes(repoID string) map[RepoPermissionScope]string {
	return map[RepoPermissionScope]string{
		RepoOnlyScope:         GetRepoOnlyScope(repoID),
		RepoAndProjectsScope:  GetRepoAdminScope(repoID),
		RepoUnrestrictedScope: GetUnrestrictedBranchProjectsScope(repoID),
	}
}

// GetRepoOnlyScope returns the scope containing just the repo settings resource.
func GetRepoOnlyScope(repoID string) string {
	return repoOnlyScopePrefix + repoID
}

// IsRepoPermissionScope identifies scopes maintained for repo-level grants.
func IsRepoPermissionScope(scopeID string) bool {
	return strings.HasPrefix(scopeID, repoOnlyScopePrefix) || strings.HasPrefix(scopeID, repoAdminScopePrefix) || strings.HasPrefix(scopeID, repoUnrestrictedScopePrefix)
}

func (p *ProjectRef) resolveRestricted(ctx context.Context) (bool, error) {
	if !p.UseRepoSettings() || p.Restricted != nil {
		return p.IsRestricted(), nil
	}
	repo, err := FindOneRepoRef(ctx, p.RepoRefId)
	if err != nil {
		return false, errors.Wrap(err, "finding inherited access restrictions")
	}
	return p.isRestrictedWithRepo(repo), nil
}

func (p *ProjectRef) isRestrictedWithRepo(repo *RepoRef) bool {
	if p.Restricted != nil {
		return p.IsRestricted()
	}
	return repo != nil && repo.IsRestricted()
}

// EnsureRepoPermissionScopes initializes missing scopes for repo-level grants.
// Existing scope membership is preserved, including concurrent updates.
func EnsureRepoPermissionScopes(ctx context.Context, rm gimlet.RoleManager, repo *RepoRef) error {
	missingScopes := map[RepoPermissionScope]string{}
	for kind, scopeID := range RepoPermissionScopes(repo.Id) {
		scope, err := rm.GetScope(ctx, scopeID)
		if err != nil {
			return errors.Wrap(err, "getting repo permission scope")
		}
		if scope == nil {
			missingScopes[kind] = scopeID
		}
	}
	if len(missingScopes) == 0 {
		return nil
	}
	resources, err := repoPermissionScopeResources(ctx, repo)
	if err != nil {
		return err
	}
	for kind, scopeID := range missingScopes {
		if err := rm.AddScope(ctx, gimlet.Scope{
			ID:        scopeID,
			Name:      repo.Id,
			Type:      evergreen.ProjectResourceType,
			Resources: resources[kind],
		}); err != nil && !mongo.IsDuplicateKeyError(err) {
			return errors.Wrap(err, "creating repo permission scope")
		}
	}
	return nil
}

// ReconcileRepoPermissionScopes repairs scope membership for a one-time migration.
// Callers must pause project attachment, detachment, and restriction changes for
// this repo until reconciliation completes. This must not run on the grant path.
func ReconcileRepoPermissionScopes(ctx context.Context, rm gimlet.RoleManager, repo *RepoRef) error {
	if err := EnsureRepoPermissionScopes(ctx, rm, repo); err != nil {
		return err
	}
	resources, err := repoPermissionScopeResources(ctx, repo)
	if err != nil {
		return err
	}
	for kind, scopeID := range RepoPermissionScopes(repo.Id) {
		scope, err := rm.GetScope(ctx, scopeID)
		if err != nil {
			return errors.Wrap(err, "getting repo permission scope")
		}
		if scope == nil {
			return errors.Errorf("repo permission scope '%s' not found", scopeID)
		}
		wanted := map[string]bool{}
		present := map[string]bool{}
		for _, resourceID := range scope.Resources {
			present[resourceID] = true
		}
		for _, resourceID := range resources[kind] {
			wanted[resourceID] = true
			if present[resourceID] {
				continue
			}
			if err := rm.AddResourceToScope(ctx, scopeID, resourceID); err != nil {
				return errors.Wrap(err, "adding resource to repo permission scope")
			}
		}
		for _, resourceID := range scope.Resources {
			if wanted[resourceID] {
				continue
			}
			if err := rm.RemoveResourceFromScope(ctx, scopeID, resourceID); err != nil {
				return errors.Wrap(err, "removing resource from repo permission scope")
			}
		}
	}
	return nil
}

func repoPermissionScopeResources(ctx context.Context, repo *RepoRef) (map[RepoPermissionScope][]string, error) {
	projects := []ProjectRef{}
	err := db.FindAllQ(ctx, ProjectRefCollection, db.Query(bson.M{ProjectRefRepoRefIdKey: repo.Id}).WithFields(ProjectRefIdKey, ProjectRefRestrictedKey), &projects)
	if err != nil {
		return nil, errors.Wrap(err, "finding attached projects for repo permissions")
	}
	resources := map[RepoPermissionScope][]string{
		RepoOnlyScope:         {repo.Id},
		RepoAndProjectsScope:  {repo.Id},
		RepoUnrestrictedScope: {},
	}
	for _, project := range projects {
		resources[RepoAndProjectsScope] = append(resources[RepoAndProjectsScope], project.Id)
		if !project.isRestrictedWithRepo(repo) {
			resources[RepoUnrestrictedScope] = append(resources[RepoUnrestrictedScope], project.Id)
		}
	}
	return resources, nil
}
