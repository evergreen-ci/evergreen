package resolvers

// This file will always be generated when running gqlgen.
// It contains the definitions for the Query & Mutation resolver, which are used by the other files in this folder.

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

type Resolver struct {
	sc data.Connector
}

func New(apiURL string) generated.Config {
	c := generated.Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
	c.Directives.RequireSuperUser = func(ctx context.Context, obj interface{}, next graphql.Resolver) (interface{}, error) {
		user := gimlet.GetUser(ctx)
		if user == nil {
			return nil, gqlError.Forbidden.Send(ctx, "user not logged in")
		}
		opts := gimlet.PermissionOpts{
			Resource:      evergreen.SuperUserPermissionsID,
			ResourceType:  evergreen.SuperUserResourceType,
			Permission:    evergreen.PermissionAdminSettings,
			RequiredLevel: evergreen.AdminSettingsEdit.Value,
		}
		if user.HasPermission(opts) {
			return next(ctx)
		}
		return nil, gqlError.Forbidden.Send(ctx, fmt.Sprintf("user %s does not have permission to access this resolver", user.Username()))
	}
	c.Directives.RequireProjectAccess = func(ctx context.Context, obj interface{}, next graphql.Resolver, access gqlModel.ProjectSettingsAccess) (res interface{}, err error) {
		var permissionLevel int
		if access == gqlModel.ProjectSettingsAccessEdit {
			permissionLevel = evergreen.ProjectSettingsEdit.Value
		} else if access == gqlModel.ProjectSettingsAccessView {
			permissionLevel = evergreen.ProjectSettingsView.Value
		} else {
			return nil, gqlError.Forbidden.Send(ctx, "Permission not specified")
		}

		args, isStringMap := obj.(map[string]interface{})
		if !isStringMap {
			return nil, gqlError.ResourceNotFound.Send(ctx, "Project not specified")
		}

		if id, hasId := args["id"].(string); hasId {
			return util.HasProjectPermission(ctx, id, next, permissionLevel)
		} else if projectId, hasProjectId := args["projectId"].(string); hasProjectId {
			return util.HasProjectPermission(ctx, projectId, next, permissionLevel)
		} else if identifier, hasIdentifier := args["identifier"].(string); hasIdentifier {
			pid, err := model.GetIdForProject(identifier)
			if err != nil {
				return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project with identifier: %s", identifier))
			}
			return util.HasProjectPermission(ctx, pid, next, permissionLevel)
		}
		return nil, gqlError.ResourceNotFound.Send(ctx, "Could not find project")
	}
	return c
}
