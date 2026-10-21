package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
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
		taskIDs = append(taskIDs, id.(string))
	}

	projectIDs, err := task.FindProjectsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding projects for tasks: %s", err.Error()))
	}

	for _, projectID := range utility.UniqueStrings(projectIDs) {
		if err = checkProjectPermission(ctx, projectID, requiredPermission, permissionInfo); err != nil {
			return nil, err
		}
	}
	return next(ctx)
}
