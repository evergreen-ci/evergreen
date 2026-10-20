package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// RepoPermissionScope selects a maintained scope for a repo-level grant.
type RepoPermissionScope string

const (
	RepoOnlyScope         RepoPermissionScope = "repo_only"
	RepoAndProjectsScope  RepoPermissionScope = "repo_and_projects"
	RepoUnrestrictedScope RepoPermissionScope = "unrestricted_branches"
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
	return fmt.Sprintf("repo_only_%s", repoID)
}

// IsRepoPermissionScope identifies scopes maintained for repo-level grants.
func IsRepoPermissionScope(scopeID string) bool {
	return strings.HasPrefix(scopeID, "repo_only_") || strings.HasPrefix(scopeID, "admin_repo_") || strings.HasPrefix(scopeID, "unrestricted_branches_")
}

func (p *ProjectRef) isRestrictedWithRepo(repo *RepoRef) bool {
	if p.Restricted != nil {
		return p.IsRestricted()
	}
	return repo != nil && repo.IsRestricted()
}

// EnsureRepoPermissionScopes initializes missing scopes and reconciles their
// membership before existing repos receive their first MANA grants.
func EnsureRepoPermissionScopes(ctx context.Context, rm gimlet.RoleManager, repo *RepoRef) error {
	projects := []ProjectRef{}
	err := db.FindAllQ(ctx, ProjectRefCollection, db.Query(bson.M{ProjectRefRepoRefIdKey: repo.Id}).WithFields(ProjectRefIdKey, ProjectRefRestrictedKey), &projects)
	if err != nil {
		return errors.Wrap(err, "finding attached projects for repo permissions")
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
	for kind, scopeID := range RepoPermissionScopes(repo.Id) {
		scope, err := rm.GetScope(ctx, scopeID)
		if err != nil {
			return errors.Wrap(err, "getting repo permission scope")
		}
		if scope == nil {
			if err := rm.AddScope(ctx, gimlet.Scope{ID: scopeID, Type: evergreen.ProjectResourceType, Resources: resources[kind]}); err != nil {
				return errors.Wrap(err, "creating repo permission scope")
			}
			continue
		}
		wanted := map[string]bool{}
		present := map[string]bool{}
		for _, resourceID := range scope.Resources {
			present[resourceID] = true
		}
		for _, resourceID := range resources[kind] {
			wanted[resourceID] = true
			if !present[resourceID] {
				if err := rm.AddResourceToScope(ctx, scopeID, resourceID); err != nil {
					return errors.Wrap(err, "adding resource to repo permission scope")
				}
			}
		}
		for _, resourceID := range scope.Resources {
			if !wanted[resourceID] {
				if err := rm.RemoveResourceFromScope(ctx, scopeID, resourceID); err != nil {
					return errors.Wrap(err, "removing resource from repo permission scope")
				}
			}
		}
	}
	return nil
}
