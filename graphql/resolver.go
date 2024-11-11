package graphql

// This file will always be generated when running gqlgen.
// werrors imports in the resolvers are due to https://github.com/99designs/gqlgen/issues/1171.

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

const (
	CreateProjectMutation   = "CreateProject"
	CopyProjectMutation     = "CopyProject"
	DeleteProjectMutation   = "DeleteProject"
	SetLastRevisionMutation = "SetLastRevision"
)

type Resolver struct {
	sc data.Connector
}

func New(apiURL string) Config {
	dbConnector := &data.DBConnector{URL: apiURL}
	c := Config{
		Resolvers: &Resolver{
			sc: dbConnector,
		},
	}
	c.Directives.RequireDistroAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver, access DistroSettingsAccess) (interface{}, error) {
		user := mustHaveUser(ctx)

		// If directive is checking for create permissions, no distro ID is required.
		if access == DistroSettingsAccessCreate {
			if userHasDistroCreatePermission(user) {
				return next(ctx)
			}
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have create distro permissions", user.Username()))
		}

		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "distro not specified")
		}
		distroId, hasDistroId := args["distroId"].(string)
		if !hasDistroId {
			name, hasName := args["name"].(string)

			if !hasName {
				return nil, ResourceNotFound.Send(ctx, "Distro not specified")
			}
			distroId = name
		}

		var requiredLevel int
		if access == DistroSettingsAccessAdmin {
			requiredLevel = evergreen.DistroSettingsAdmin.Value
		} else if access == DistroSettingsAccessEdit {
			requiredLevel = evergreen.DistroSettingsEdit.Value
		} else if access == DistroSettingsAccessView {
			requiredLevel = evergreen.DistroSettingsView.Value
		} else {
			return nil, Forbidden.Send(ctx, "Permission not specified")
		}

		if userHasDistroPermission(user, distroId, requiredLevel) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access settings for the distro '%s'", user.Username(), distroId))
	}
	c.Directives.RequireProjectAdmin = func(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
		// Allow if user is superuser.
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

		operationContext := graphql.GetOperationContext(ctx).OperationName

		if operationContext == CreateProjectMutation {
			canCreate, err := user.HasProjectCreatePermission()
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("checking user permissions: %s", err.Error()))
			}
			if canCreate {
				return next(ctx)
			}
		}

		getPermissionOpts := func(projectId string) gimlet.PermissionOpts {
			return gimlet.PermissionOpts{
				Resource:      projectId,
				ResourceType:  evergreen.ProjectResourceType,
				Permission:    evergreen.PermissionProjectSettings,
				RequiredLevel: evergreen.ProjectSettingsEdit.Value,
			}
		}

		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "Project not specified")
		}

		if operationContext == CopyProjectMutation {
			projectIdToCopy, ok := args["project"].(map[string]interface{})["projectIdToCopy"].(string)
			if !ok {
				return nil, InternalServerError.Send(ctx, "finding projectIdToCopy for copy project operation")
			}
			opts := getPermissionOpts(projectIdToCopy)
			if user.HasPermission(opts) {
				return next(ctx)
			}
		}

		if operationContext == DeleteProjectMutation {
			projectId, ok := args["projectId"].(string)
			if !ok {
				return nil, InternalServerError.Send(ctx, "finding projectId for delete project operation")
			}
			opts := getPermissionOpts(projectId)
			if user.HasPermission(opts) {
				return next(ctx)
			}
		}

		if operationContext == SetLastRevisionMutation {
			projectIdentifier, ok := args["opts"].(map[string]interface{})["projectIdentifier"].(string)
			if !ok {
				return nil, InternalServerError.Send(ctx, "finding projectIdentifier for set last revision operation")
			}
			project, err := model.FindBranchProjectRef(projectIdentifier)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding project '%s': %s", projectIdentifier, err.Error()))
			}
			if project == nil {
				return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectIdentifier))
			}
			opts := getPermissionOpts(project.Id)
			if user.HasPermission(opts) {
				return next(ctx)
			}
		}

		return nil, Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access the %s resolver", user.Username(), operationContext))
	}
	c.Directives.RequireProjectAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver, permission ProjectPermission, access AccessLevel) (interface{}, error) {
		usr := mustHaveUser(ctx)

		args, isMap := obj.(map[string]interface{})
		if !isMap {
			return nil, InternalServerError.Send(ctx, "converting args into map")
		}

		requiredPermission, permissionInfo, err := getProjectPermissionLevel(permission, access)
		if err != nil {
			return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid permission and access level configuration: %s", err.Error()))
		}

		paramsMap, err := data.BuildProjectParameterMapForGraphQL(args)
		if err != nil {
			return nil, InputValidationError.Send(ctx, err.Error())
		}

		projectId, statusCode, err := data.GetProjectIdFromParams(ctx, paramsMap)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, statusCode, err)
		}

		hasPermission := usr.HasPermission(gimlet.PermissionOpts{
			Resource:      projectId,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    requiredPermission,
			RequiredLevel: permissionInfo.Value,
		})
		if hasPermission {
			return next(ctx)
		}

		if requiredPermission == evergreen.PermissionProjectSettings && permissionInfo.Value == evergreen.ProjectSettingsView.Value {
			// If we're trying to view a repo project, check if the user has view permission for any branch project instead.
			hasPermission, err = model.UserHasRepoViewPermission(usr, projectId)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem checking repo view permission: %s", err.Error()))
			}
			if hasPermission {
				return next(ctx)
			}
		}

		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to '%s' for the project '%s'", usr.Username(), strings.ToLower(permissionInfo.Description), projectId))
	}
	c.Directives.RequireProjectSettingsAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver) (res interface{}, err error) {
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

		hasPermission := usr.HasPermission(gimlet.PermissionOpts{
			Resource:      projectId,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsView.Value,
		})
		if hasPermission {
			return next(ctx)
		}

		// In case this is a repo project, check if the user has view permission for any branch project instead.
		hasPermission, err = model.UserHasRepoViewPermission(usr, projectId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem checking repo view permission: %s", err.Error()))
		}
		if hasPermission {
			return next(ctx)
		}

		return nil, Forbidden.Send(ctx, fmt.Sprintf("user does not have permission to access the field '%s' for project with ID '%s'", graphql.GetFieldContext(ctx).Path(), projectId))
	}
	c.Directives.RedactSecrets = func(ctx context.Context, obj interface{}, next graphql.Resolver) (res interface{}, err error) {
		return next(ctx)
	}
	return c
}
