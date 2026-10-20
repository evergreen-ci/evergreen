package route

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// Repo grants select project-typed scopes, preserving existing authorization checks.
const repoResourceType = "repo"

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
	nonzero := gimlet.Permissions{}
	for permission, value := range permissions {
		if value != 0 {
			nonzero[permission] = value
		}
	}
	if !maps.Equal(nonzero, expected) {
		return errors.New("permissions do not match the selected repo capability")
	}
	return nil
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
		return nil, err
	}
	permissions := gimlet.Permissions{}
	for permission, value := range request.Permissions {
		if value != 0 {
			permissions[permission] = value
		}
	}
	scopeID := model.RepoPermissionScopes(repo.Id)[request.RepoScope]
	encoded, err := json.Marshal(permissions)
	if err != nil {
		return nil, errors.Wrap(err, "encoding repo permissions")
	}
	// Include the scope identity so roles remain distinct even when two scopes
	// temporarily contain the same resources.
	roleID := fmt.Sprintf("mana_repo_%x", sha256.Sum256(append([]byte(scopeID+":"), encoded...)))
	roles, err := rm.GetRoles(ctx, []string{roleID})
	if err != nil {
		return nil, errors.Wrap(err, "finding repo permission role")
	}
	if len(roles) > 0 {
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

func getRepoUserPermissions(ctx context.Context, rm gimlet.RoleManager, repoID string) gimlet.Responder {
	repo, err := model.FindOneRepoRef(ctx, repoID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding repo"))
	}
	if repo == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{StatusCode: http.StatusNotFound, Message: "repo not found"})
	}
	roles, err := rm.GetAllRoles(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting repo roles"))
	}
	scopeKinds := map[string]model.RepoPermissionScope{}
	for kind, id := range model.RepoPermissionScopes(repoID) {
		scopeKinds[id] = kind
	}
	roleIDs := []string{}
	selected := map[string]gimlet.Role{}
	for _, role := range roles {
		if _, ok := scopeKinds[role.Scope]; ok {
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
