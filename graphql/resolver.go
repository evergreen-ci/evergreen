package graphql

// This file will always be generated when running gqlgen.
// werrors imports in the resolvers are due to https://github.com/99designs/gqlgen/issues/1171.

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
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
		user := gimlet.GetUser(ctx)
		if user == nil {
			return nil, Forbidden.Send(ctx, "user not logged in")
		}
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

		if id, hasId := args["id"].(string); hasId {
			return hasProjectPermission(ctx, id, next, permissionLevel)
		} else if projectId, hasProjectId := args["projectId"].(string); hasProjectId {
			return hasProjectPermission(ctx, projectId, next, permissionLevel)
		} else if identifier, hasIdentifier := args["identifier"].(string); hasIdentifier {
			pid, err := model.GetIdForProject(identifier)
			if err != nil {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project with identifier: %s", identifier))
			}
			return hasProjectPermission(ctx, pid, next, permissionLevel)
		}
		return nil, ResourceNotFound.Send(ctx, "Could not find project")
	}
	c.Directives.RestrictProjectAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver) (res interface{}, err error) {
		user := gimlet.GetUser(ctx)
		if user == nil {
			return nil, Forbidden.Send(ctx, "user not logged in")
		}

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
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user does not have permission to access the field %s for project with ID '%s'", graphql.GetFieldContext(ctx).Path(), projectId))
	}
	return c
}
