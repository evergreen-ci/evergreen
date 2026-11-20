package graphql

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/task"
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
	rawTaskIDs, ok := args["taskIds"].([]any)
	if !ok {
		return nil, InputValidationError.Send(ctx, "must specify 'taskIds'")
	}
	taskIDs := make([]string, 0, len(rawTaskIDs))
	for _, id := range rawTaskIDs {
		tId, ok := id.(string)
		if !ok {
			return nil, InputValidationError.Send(ctx, "taskIds must be strings")
		}
		taskIDs = append(taskIDs, tId)
	}

	projectIDs, notFoundTaskIDs, err := task.FindProjectsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding projects for tasks: %s", err.Error()))
	}
	if len(notFoundTaskIDs) > 0 {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("tasks not found: '%s'", strings.Join(notFoundTaskIDs, "', '")))
	}

	for _, projectID := range projectIDs {
		if err = checkProjectPermission(ctx, projectID, requiredPermission, permissionInfo); err != nil {
			return nil, err
		}
	}
	return next(ctx)
}
