package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/rest/data"
)

// RequireProjectAccessForTasks is similar to RequireProjectAccess, except that it checks the project permissions
// for every task in the taskIds argument.
func RequireProjectAccessForTasks(ctx context.Context, obj any, next graphql.Resolver, permission ProjectPermission, access AccessLevel) (any, error) {
	requiredPermission, permissionInfo, err := getProjectPermissionLevel(permission, access)
	if err != nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid permission and access level configuration: %s", err.Error()))
	}

	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, InternalServerError.Send(ctx, "converting args into map")
	}
	taskIDs, ok := args["taskIds"].([]any)
	if !ok {
		return nil, InputValidationError.Send(ctx, "must specify 'taskIds'")
	}

	seenProjectIDs := map[string]bool{}
	for _, taskID := range taskIDs {
		projectID, statusCode, err := data.GetProjectIdFromParams(ctx, map[string]string{"taskId": taskID.(string)})
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
	return next(ctx)
}
