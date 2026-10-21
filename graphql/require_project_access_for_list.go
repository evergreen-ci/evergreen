package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/rest/data"
)

var listArgToParamKey = map[string]string{
	"taskIds":    "taskId",
	"versionIds": "versionId",
	"patchIds":   "patchId",
}

// RequireProjectAccessForList is the list-argument counterpart of RequireProjectAccess.
// It will check permissions for each given ID in the list arguments (e.g. taskIds, versionIds, patchIds).
func RequireProjectAccessForList(ctx context.Context, obj any, next graphql.Resolver, permission ProjectPermission, access AccessLevel) (any, error) {
	requiredPermission, permissionInfo, err := getProjectPermissionLevel(permission, access)
	if err != nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid permission and access level configuration: %s", err.Error()))
	}

	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, InternalServerError.Send(ctx, "converting args into map")
	}

	foundListArg := false
	seenProjectIDs := map[string]bool{}
	for argKey, paramKey := range listArgToParamKey {
		ids, ok := args[argKey].([]any)
		if !ok {
			continue
		}
		foundListArg = true

		for _, id := range ids {
			projectID, statusCode, err := data.GetProjectIdFromParams(ctx, map[string]string{paramKey: id.(string)})
			if err != nil {
				return nil, mapHTTPStatusToGqlError(ctx, statusCode, err)
			}
			if seenProjectIDs[projectID] {
				continue
			}
			if err = checkProjectPermission(ctx, projectID, requiredPermission, permissionInfo); err != nil {
				return nil, err
			}
			seenProjectIDs[projectID] = true
		}
	}
	if !foundListArg {
		return nil, InputValidationError.Send(ctx, "must specify at least one of 'taskIds', 'versionIds', or 'patchIds'")
	}
	return next(ctx)
}
