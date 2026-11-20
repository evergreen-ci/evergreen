package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

// RequireProjectSettingsAccess requires project settings view permission to resolve a field on a ProjectSettings
// (or RepoSettings) object. For repos, view permission on any branch project is also sufficient.
func RequireProjectSettingsAccess(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
	usr := mustHaveUser(ctx)

	projectSettings, isProjectSettings := obj.(*restModel.APIProjectSettings)
	if !isProjectSettings {
		return nil, InternalServerError.Send(ctx, "project not valid")
	}

	projectRef := projectSettings.ProjectRef
	projectId := utility.FromStringPtr(projectRef.Id)
	if projectId == "" {
		return nil, ResourceNotFound.Send(ctx, "project not specified")
	}

	hasPermission := usr.HasPermission(ctx, gimlet.PermissionOpts{
		Resource:      projectId,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsView.Value,
	})
	if hasPermission {
		return next(ctx)
	}

	// In case this is a repo project, check if the user has view permission for any branch project instead.
	hasPermission, err = model.UserHasRepoViewPermission(ctx, usr, projectId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem checking repo view permission: %s", err.Error()))
	}
	if hasPermission {
		return next(ctx)
	}

	return nil, Forbidden.Send(ctx, fmt.Sprintf("user does not have permission to access the field '%s' for project with ID '%s'", graphql.GetFieldContext(ctx).Path(), projectId))
}
