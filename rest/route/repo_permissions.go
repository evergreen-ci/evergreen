package route

import (
	"context"
	"maps"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// Repo grants select project-typed scopes, preserving existing authorization checks.
const repoResourceType = "repo"

const manaRepoRolePrefix = "mana_repo_"

// isRepoPermissionRole identifies repo grants without reclassifying legacy
// project grants that reused a maintained repo scope.
func isRepoPermissionRole(role gimlet.Role) bool {
	return model.IsRepoPermissionScope(role.Scope) &&
		strings.HasPrefix(role.ID, manaRepoRolePrefix)
}

func validateRepoPermissions(scope model.RepoPermissionScope, permissions gimlet.Permissions) error {
	var expected gimlet.Permissions
	switch scope {
	case model.RepoOnlyScope:
		expected = gimlet.Permissions{evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value}
	case model.RepoUnrestrictedScope:
		expected = gimlet.Permissions{
			evergreen.PermissionTasks:   evergreen.TasksBasic.Value,
			evergreen.PermissionPatches: evergreen.PatchSubmit.Value,
			evergreen.PermissionLogs:    evergreen.LogsView.Value,
		}
	case model.RepoAndProjectsScope:
		expected = gimlet.Permissions{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
			evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
			evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
		}
	default:
		return errors.New("repo_scope must be repo_only, unrestricted_branches, or repo_and_projects")
	}
	if !maps.Equal(nonzeroPermissions(permissions), expected) {
		return errors.New("permissions do not match the selected repo capability")
	}
	return nil
}

func nonzeroPermissions(permissions gimlet.Permissions) gimlet.Permissions {
	nonzero := gimlet.Permissions{}
	for permission, value := range permissions {
		if value != 0 {
			nonzero[permission] = value
		}
	}
	return nonzero
}

func makeRepoPermissionRole(ctx context.Context, rm gimlet.RoleManager, request RequestedPermissions) (*gimlet.Role, error) {
	repo, err := model.FindOneRepoRef(ctx, request.Resources[0])
	if err != nil {
		return nil, errors.Wrap(err, "finding repo for permission grant")
	}
	if repo == nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "repo not found"}
	}
	if err := model.EnsureRepoPermissionScopes(ctx, rm, repo); err != nil {
		return nil, errors.Wrapf(err, "ensuring permission scopes for repo '%s'", repo.Id)
	}
	permissions := nonzeroPermissions(request.Permissions)
	scopeID := model.RepoPermissionScopes(repo.Id)[request.RepoScope]
	roleID := manaRepoRolePrefix + scopeID
	roles, err := rm.GetRoles(ctx, []string{roleID})
	if err != nil {
		return nil, errors.Wrap(err, "finding repo permission role")
	}
	if len(roles) == 1 {
		if roles[0].Scope != scopeID || !maps.Equal(roles[0].Permissions, permissions) {
			return nil, errors.New("existing repo permission role does not match requested access")
		}
		return &roles[0], nil
	}
	role := &gimlet.Role{ID: roleID, Scope: scopeID, Permissions: permissions}
	if err := rm.UpdateRole(ctx, *role); err != nil {
		return nil, errors.Wrap(err, "creating repo permission role")
	}
	return role, nil
}
