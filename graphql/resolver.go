package graphql

// This file will always be generated when running gqlgen.
// werrors imports in the resolvers are due to https://github.com/99designs/gqlgen/issues/1171.

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

type Resolver struct {
	sc data.Connector
}

func New(apiURL string) Config {
	c := Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
	c.Directives.RequireSuperUser = func(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
		user := mustHaveUser(ctx)
		opts := gimlet.PermissionOpts{
			Resource:      evergreen.SuperUserPermissionsID,
			ResourceType:  evergreen.SuperUserResourceType,
			Permission:    evergreen.PermissionProjectCreate,
			RequiredLevel: evergreen.ProjectCreate.Value,
		}
		if user.HasPermission(opts) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access this resolver", user.Username()))
	}
	c.Directives.RequireProjectAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver, access ProjectSettingsAccess) (res interface{}, err error) {
		user := mustHaveUser(ctx)

		var permissionLevel int
		if access == ProjectSettingsAccessEdit {
			permissionLevel = evergreen.ProjectSettingsEdit.Value
		} else if access == ProjectSettingsAccessView {
			permissionLevel = evergreen.ProjectSettingsView.Value
		} else {
			return nil, Forbidden.Send(ctx, "Permission not specified")
		}

		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "Project not specified")
		}

		projectId, err := getProjectIdFromArgs(ctx, args)
		if err != nil {
			return nil, err
		}

		opts := gimlet.PermissionOpts{
			Resource:      projectId,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: permissionLevel,
		}
		if user.HasPermission(opts) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access settings for the project %s", user.Username(), projectId))
	}
	c.Directives.RequireProjectFieldAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver) (res interface{}, err error) {
		user := mustHaveUser(ctx)

		projectRef, isProjectRef := obj.(*restModel.APIProjectRef)
		if !isProjectRef {
			return nil, InternalServerError.Send(ctx, "project not valid")
		}

		projectId := utility.FromStringPtr(projectRef.Id)
		if projectId == "" {
			return nil, ResourceNotFound.Send(ctx, "project not specified")
		}

		opts := gimlet.PermissionOpts{
			Resource:      projectId,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsView.Value,
		}
		if user.HasPermission(opts) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user does not have permission to access the field '%s' for project with ID '%s'", graphql.GetFieldContext(ctx).Path(), projectId))
	}
	return c
}
