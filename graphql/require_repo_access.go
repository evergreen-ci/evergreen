package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
)

// RequireRepoAccess requires admin or view access on the repo that the project given by the projectId argument belongs to.
// If a repo does not exist yet, project admin access is required instead.
func RequireRepoAccess(ctx context.Context, obj any, next graphql.Resolver, access AccessLevel) (any, error) {
	usr := mustHaveUser(ctx)

	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, InternalServerError.Send(ctx, "converting args into map")
	}
	projectId, hasProjectId := args["projectId"].(string)
	if !hasProjectId || projectId == "" {
		return nil, ResourceNotFound.Send(ctx, "project not specified")
	}

	pRef, err := model.FindBranchProjectRef(ctx, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding project '%s': %s", projectId, err.Error()))
	}
	if pRef == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectId))
	}

	repoRef, err := model.FindRepoRefByOwnerAndRepo(ctx, pRef.Owner, pRef.Repo)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding repo for '%s/%s': %s", pRef.Owner, pRef.Repo, err.Error()))
	}

	// When no repo exists yet, a project admin may attach the project and
	// becomes the admin of the newly created repo.
	if repoRef == nil {
		if usr.HasPermission(ctx, gimlet.PermissionOpts{
			Resource:      pRef.Id,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsEdit.Value,
		}) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' must be an admin of project '%s' to attach it to a new repo", usr.Username(), projectId))
	}

	switch access {
	case AccessLevelAdmin:
		if usr.HasPermission(ctx, gimlet.PermissionOpts{
			Resource:      repoRef.Id,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsEdit.Value,
		}) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' is not an admin of repo '%s/%s'", usr.Username(), pRef.Owner, pRef.Repo))
	case AccessLevelView:
		hasViewPermission, err := model.UserHasRepoViewPermission(ctx, usr, repoRef.Id)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("checking repo view permission for repo '%s/%s': %s", pRef.Owner, pRef.Repo, err.Error()))
		}
		if hasViewPermission {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to view repo '%s/%s'", usr.Username(), pRef.Owner, pRef.Repo))
	default:
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid access level '%s' for repo", access))
	}
}
