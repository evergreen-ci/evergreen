package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

const defaultTaskQuarantinedTestsSampleLimit = 50

func requireVersionTasksViewPermission(ctx context.Context, versionID string) error {
	params, err := data.BuildProjectParameterMapForGraphQL(map[string]any{"versionId": versionID})
	if err != nil {
		return InputValidationError.Send(ctx, err.Error())
	}
	projectID, statusCode, err := data.GetProjectIdFromParams(ctx, params)
	if err != nil {
		return mapHTTPStatusToGqlError(ctx, statusCode, err)
	}

	usr := mustHaveUser(ctx)
	if usr.HasPermission(ctx, gimlet.PermissionOpts{
		Resource:      projectID,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionTasks,
		RequiredLevel: evergreen.TasksView.Value,
	}) {
		return nil
	}
	return Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to 'view tasks' for the project '%s'", usr.Username(), projectID))
}
