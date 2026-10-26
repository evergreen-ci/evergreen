package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

func RequireProjectAccess(ctx context.Context, obj any, next graphql.Resolver, permission ProjectPermission, access AccessLevel) (any, error) {
	requiredPermission, permissionInfo, err := getProjectPermissionLevel(permission, access)
	if err != nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid permission and access level configuration: %s", err.Error()))
	}

	var projectID string
	switch typedObj := obj.(type) {
	case *restModel.APITask:
		projectID = utility.FromStringPtr(typedObj.ProjectId)
	case map[string]any:
		paramsMap, err := data.BuildProjectParameterMapForGraphQL(typedObj)
		if err != nil {
			return nil, InputValidationError.Send(ctx, err.Error())
		}
		var statusCode int
		projectID, statusCode, err = data.GetProjectIdFromParams(ctx, paramsMap)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, statusCode, err)
		}
	default:
		return nil, InternalServerError.Send(ctx, "converting args into map")
	}

	if err = checkProjectPermission(ctx, projectID, requiredPermission, permissionInfo); err != nil {
		return nil, err
	}
	return next(ctx)
}
