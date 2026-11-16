package route

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
)

// projectGrantRoleManager prevents resource-based reuse from turning a project
// grant into a maintained repo grant when their resource lists happen to match.
type projectGrantRoleManager struct{ gimlet.RoleManager }

func (m projectGrantRoleManager) FindRoleWithPermissions(ctx context.Context, resourceType string, resources []string, permissions gimlet.Permissions) (*gimlet.Role, error) {
	role, err := m.RoleManager.FindRoleWithPermissions(ctx, resourceType, resources, permissions)
	if err != nil || role == nil || !model.IsRepoPermissionScope(role.Scope) {
		return role, err
	}
	roles, err := m.FindRolesWithResources(ctx, resourceType, resources)
	if err != nil {
		return nil, err
	}
	for _, candidate := range roles {
		if model.IsRepoPermissionScope(candidate.Scope) {
			continue
		}
		if len(permissions) == 0 && candidate.Permissions != nil {
			continue
		}
		matches := true
		for key, value := range permissions {
			if level, ok := candidate.Permissions[key]; !ok || level != value {
				matches = false
				break
			}
		}
		if matches {
			return &candidate, nil
		}
	}
	return nil, nil
}

func (m projectGrantRoleManager) FindScopeForResources(ctx context.Context, resourceType string, resources ...string) (*gimlet.Scope, error) {
	scope, err := m.RoleManager.FindScopeForResources(ctx, resourceType, resources...)
	if err != nil || scope == nil || !model.IsRepoPermissionScope(scope.ID) {
		return scope, err
	}
	// Let the role helper create an immutable project scope instead.
	return nil, nil
}

func makeProjectPermissionRole(ctx context.Context, rm gimlet.RoleManager, request RequestedPermissions) (*gimlet.Role, error) {
	if request.ResourceType == evergreen.ProjectResourceType {
		rm = projectGrantRoleManager{rm}
	}
	return rolemanager.MakeRoleWithPermissions(ctx, rm, request.ResourceType, request.Resources, request.Permissions)
}
