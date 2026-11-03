package route

import (
	"context"
	"maps"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
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

// isNativeRepoAdminRole identifies the admin role maintained by a repo's admin
// list, which shares its ID with the repo admin scope.
func isNativeRepoAdminRole(role gimlet.Role) bool {
	return role.ID == role.Scope && role.ID == model.GetRepoAdminRole(strings.TrimPrefix(role.ID, "admin_repo_"))
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

// RepoUserPermissions reports grants by scope instead of flattening branch and
// repo permissions into a single effective permission map.
type RepoUserPermissions map[string]map[model.RepoPermissionScope]gimlet.Permissions

//lint:ignore U1000 Swagger-only type, included because this API route returns an external type
type swaggerRepoUserPermissions map[string]swaggerPermissions

//lint:ignore U1000 Swagger-only type, included because this API route returns an external type
type swaggerRepoUsersPermissionsResult map[string]swaggerRepoUserPermissions

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/repos/{repo_id}/permissions/users

type repoUsersPermissionsGetHandler struct {
	rm     gimlet.RoleManager
	repoID string
}

func makeGetRepoUsersPermissions(rm gimlet.RoleManager) gimlet.RouteHandler {
	return &repoUsersPermissionsGetHandler{rm: rm}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get all user permissions for repo
//	@Description	Retrieves all users with repo grants or native repo admin access, and returns their highest permissions for each repo capability (repo_only, unrestricted_branches, repo_and_projects). Native repo admins are reported under repo_and_projects.
//	@Tags			repos
//	@Router			/repos/{repo_id}/permissions/users [get]
//	@Security		Api-User || Api-Key
//	@Param			repo_id	path		string	true	"the repo ID"
//	@Success		200		{object}	swaggerRepoUsersPermissionsResult
func (h *repoUsersPermissionsGetHandler) Factory() gimlet.RouteHandler {
	return &repoUsersPermissionsGetHandler{rm: h.rm}
}

func (h *repoUsersPermissionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.repoID = gimlet.GetVars(r)["repo_id"]
	return nil
}

func (h *repoUsersPermissionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	repo, err := model.FindOneRepoRef(ctx, h.repoID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding repo '%s'", h.repoID))
	}
	if repo == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("repo '%s' not found", h.repoID)})
	}
	roles, err := h.rm.GetAllRoles(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting repo roles"))
	}
	scopeKinds := map[string]model.RepoPermissionScope{}
	for kind, id := range model.RepoPermissionScopes(repo.Id) {
		scopeKinds[id] = kind
	}
	roleIDs := []string{}
	selected := map[string]gimlet.Role{}
	for _, role := range roles {
		// Native repo admins are listed like project admins are for projects, even
		// though they can only be removed through the repo's admin list.
		if _, ok := scopeKinds[role.Scope]; ok && (isRepoPermissionRole(role) || isNativeRepoAdminRole(role)) {
			roleIDs = append(roleIDs, role.ID)
			selected[role.ID] = role
		}
	}
	users, err := user.FindHumanUsersByRoles(ctx, roleIDs)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding repo users"))
	}
	result := RepoUserPermissions{}
	for _, u := range users {
		result[u.Id] = map[model.RepoPermissionScope]gimlet.Permissions{}
		for _, roleID := range u.SystemRoles {
			if role, ok := selected[roleID]; ok {
				kind := scopeKinds[role.Scope]
				result[u.Id][kind] = getMaxPermissions(result[u.Id][kind], role.Permissions)
			}
		}
	}
	return gimlet.NewJSONResponse(result)
}
