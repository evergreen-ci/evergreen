package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/rest/data"
)

func RequireProjectAccess(ctx context.Context, obj any, next graphql.Resolver, permission ProjectPermission, access AccessLevel) (any, error) {
	requiredPermission, permissionInfo, err := getProjectPermissionLevel(permission, access)
	if err != nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid permission and access level configuration: %s", err.Error()))
	}

	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, InternalServerError.Send(ctx, "converting args into map")
	}

	paramsMap, err := data.BuildProjectParameterMapForGraphQL(args)
	if err != nil {
		return nil, InputValidationError.Send(ctx, err.Error())
	}
	projectID, statusCode, err := data.GetProjectIdFromParams(ctx, paramsMap)
	if err != nil {
		return nil, mapHTTPStatusToGqlError(ctx, statusCode, err)
	}

	if err = checkProjectPermission(ctx, projectID, requiredPermission, permissionInfo); err != nil {
		return nil, err
	}
	return next(ctx)
}
