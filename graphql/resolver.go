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

const (
	CreateProjectMutation = "CreateProject"
	CopyProjectMutation   = "CopyProject"
	DeleteProjectMutation = "DeleteProject"
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
			opts := gimlet.PermissionOpts{
				Resource:      evergreen.SuperUserPermissionsID,
				ResourceType:  evergreen.SuperUserResourceType,
				Permission:    evergreen.PermissionDistroCreate,
				RequiredLevel: evergreen.DistroCreate.Value,
			}
			if user.HasPermission(opts) {
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

		// Check for admin permissions for each of the resolvers.
		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "Project not specified")
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

		if operationContext == CopyProjectMutation {
			projectIdToCopy, ok := args["project"].(map[string]interface{})["projectIdToCopy"].(string)
			if !ok {
				return nil, InternalServerError.Send(ctx, "finding projectIdToCopy for copy project operation")
			}
			opts := gimlet.PermissionOpts{
				Resource:      projectIdToCopy,
				ResourceType:  evergreen.ProjectResourceType,
				Permission:    evergreen.PermissionProjectSettings,
				RequiredLevel: evergreen.ProjectSettingsEdit.Value,
			}
			if user.HasPermission(opts) {
				return next(ctx)
			}
		}

		if operationContext == DeleteProjectMutation {
			projectId, ok := args["projectId"].(string)
			if !ok {
				return nil, InternalServerError.Send(ctx, "finding projectId for delete project operation")
			}
			opts := gimlet.PermissionOpts{
				Resource:      projectId,
				ResourceType:  evergreen.ProjectResourceType,
				Permission:    evergreen.PermissionProjectSettings,
				RequiredLevel: evergreen.ProjectSettingsEdit.Value,
			}
			if user.HasPermission(opts) {
				return next(ctx)
			}
		}

		return nil, Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access the %s resolver", user.Username(), operationContext))
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
	c.Directives.RequireProjectSettingsAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver) (res interface{}, err error) {
		user := mustHaveUser(ctx)

		projectSettings, isProjectSettings := obj.(*restModel.APIProjectSettings)
		if !isProjectSettings {
			return nil, InternalServerError.Send(ctx, "project not valid")
		}

		projectRef := projectSettings.ProjectRef
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
	c.Directives.RequireCommitQueueItemOwner = func(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
		usr := mustHaveUser(ctx)

		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, InternalServerError.Send(ctx, "converting mutation args into map")
		}

		commitQueueId, hasCommitQueueId := args["commitQueueId"].(string)
		if !hasCommitQueueId {
			return nil, InputValidationError.Send(ctx, "commit queue id was not provided")
		}

		issue, hasIssue := args["issue"].(string)
		if !hasIssue {
			return nil, InputValidationError.Send(ctx, "issue was not provided")
		}

		project, err := data.FindProjectById(commitQueueId, true, false)
		if err != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}

		if err = data.CheckCanRemoveCommitQueueItem(ctx, dbConnector, usr, project, issue); err != nil {
			gimletErr, ok := err.(gimlet.ErrorResponse)
			if ok {
				return nil, mapHTTPStatusToGqlError(ctx, gimletErr.StatusCode, err)
			}
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		return next(ctx)
	}
	return c
}
