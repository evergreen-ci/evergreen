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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
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
	c.Directives.RequirePatchOwner = func(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
		user := mustHaveUser(ctx)
		args, isStringMap := obj.(map[string]any)
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "patchIds not specified")
		}
		rawPatchIds, hasPatchIds := args["patchIds"].([]any)
		if !hasPatchIds {
			return nil, ResourceNotFound.Send(ctx, "patchIds not specified")
		}
		patchIds := make([]string, len(rawPatchIds))
		for i, v := range rawPatchIds {
			patchIds[i] = v.(string)
		}

		patches, err := patch.Find(ctx, patch.ByStringIds(patchIds))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches '%s': %s", patchIds, err.Error()))
		}

		forbiddenPatches := []string{}
		for _, p := range patches {
			if !userCanModifyPatch(user, p) {
				forbiddenPatches = append(forbiddenPatches, p.Id.Hex())
			}
		}
		if len(forbiddenPatches) == 1 {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patch '%s'", user.Username(), forbiddenPatches[0]))
		} else if len(forbiddenPatches) > 1 {
			patchString := strings.Join(forbiddenPatches, ", ")
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patches: '%s'", user.Username(), patchString))
		}

		return next(ctx)
	}
	c.Directives.RequireHostAccess = func(ctx context.Context, obj any, next graphql.Resolver, access HostAccessLevel) (any, error) {
		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "host not specified")
		}
		hostId, hasHostId := args["hostId"].(string)
		hostIdsInterface, hasHostIds := args["hostIds"].([]interface{})
		if !hasHostId && !hasHostIds {
			return nil, ResourceNotFound.Send(ctx, "host not specified")
		}

		hostIdsToCheck := []string{hostId}
		if hasHostIds {
			for _, v := range hostIdsInterface {
				hostIdsToCheck = append(hostIdsToCheck, v.(string))
			}
		}
		var requiredLevel int
		if access == HostAccessLevelEdit {
			requiredLevel = evergreen.HostsEdit.Value
		} else {
			requiredLevel = evergreen.HostsView.Value
		}
		user := mustHaveUser(ctx)
		hostsToCheck, err := host.Find(ctx, host.ByIds(hostIdsToCheck))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting hosts: %s", err.Error()))
		}
		if len(hostsToCheck) == 0 {
			return nil, ResourceNotFound.Send(ctx, "No matching hosts found")
		}
		forbiddenHosts := []string{}
		for _, h := range hostsToCheck {
			if !userHasHostPermission(user, h.Distro.Id, requiredLevel, h.StartedBy) {
				forbiddenHosts = append(forbiddenHosts, h.Id)
			}
		}
		if len(forbiddenHosts) == 1 {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access host '%s'", user.Username(), forbiddenHosts[0]))
		} else if len(forbiddenHosts) > 1 {
			hostsString := strings.Join(forbiddenHosts, ", ")
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access hosts: '%s'", user.Username(), hostsString))
		}

		return next(ctx)
	}
	c.Directives.RequireDistroAccess = func(ctx context.Context, obj any, next graphql.Resolver, access DistroSettingsAccess) (any, error) {
		user := mustHaveUser(ctx)

		// If directive is checking for create permissions, no distro ID is required.
		if access == DistroSettingsAccessCreate {

			if user.HasDistroCreatePermission() {
				return next(ctx)
			}
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have create distro permissions", user.Username()))
		}

		args, isStringMap := obj.(map[string]any)
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
	c.Directives.RequireProjectAdmin = func(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
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

		args, isStringMap := obj.(map[string]any)
		if !isStringMap {
			return nil, ResourceNotFound.Send(ctx, "Project not specified")
		}

		if operationContext == CopyProjectMutation {
			projectIdToCopy, ok := args["project"].(map[string]any)["projectIdToCopy"].(string)
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
			projectIdentifier, ok := args["opts"].(map[string]any)["projectIdentifier"].(string)
			if !ok {
				return nil, InternalServerError.Send(ctx, "finding projectIdentifier for set last revision operation")
			}
			project, err := model.FindBranchProjectRef(ctx, projectIdentifier)
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
	c.Directives.RequireProjectAccess = func(ctx context.Context, obj any, next graphql.Resolver, permission ProjectPermission, access AccessLevel) (any, error) {
		usr := mustHaveUser(ctx)

		args, isMap := obj.(map[string]any)
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
			hasPermission, err = model.UserHasRepoViewPermission(ctx, usr, projectId)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem checking repo view permission: %s", err.Error()))
			}
			if hasPermission {
				return next(ctx)
			}
		}

		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to '%s' for the project '%s'", usr.Username(), strings.ToLower(permissionInfo.Description), projectId))
	}
	c.Directives.RequireProjectSettingsAccess = func(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
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
		hasPermission, err = model.UserHasRepoViewPermission(ctx, usr, projectId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem checking repo view permission: %s", err.Error()))
		}
		if hasPermission {
			return next(ctx)
		}

		return nil, Forbidden.Send(ctx, fmt.Sprintf("user does not have permission to access the field '%s' for project with ID '%s'", graphql.GetFieldContext(ctx).Path(), projectId))
	}
	c.Directives.RedactSecrets = func(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
		return next(ctx)
	}
	c.Directives.RequireAdmin = func(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
		dbUser := mustHaveUser(ctx)

		permissions := gimlet.PermissionOpts{
			Resource:      evergreen.SuperUserPermissionsID,
			ResourceType:  evergreen.SuperUserResourceType,
			Permission:    evergreen.PermissionAdminSettings,
			RequiredLevel: evergreen.AdminSettingsEdit.Value,
		}

		if dbUser.HasPermission(permissions) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("User '%s' lacks required admin permissions", dbUser.Username()))
	}

	return c
}
