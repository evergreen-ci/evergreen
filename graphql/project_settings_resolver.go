package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

func (r *projectSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return getAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *projectSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
	hook, err := model.FindGithubHook(utility.FromStringPtr(obj.ProjectRef.Owner), utility.FromStringPtr(obj.ProjectRef.Repo))
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Database error finding github hook for project '%s': %s", *obj.ProjectRef.Id, err.Error()))
	}
	return hook != nil, nil
}

func (r *projectSettingsResolver) Subscriptions(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

func (r *projectSettingsResolver) Vars(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	return getRedactedAPIVarsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// ProjectSettings returns ProjectSettingsResolver implementation.
func (r *Resolver) ProjectSettings() ProjectSettingsResolver { return &projectSettingsResolver{r} }

type projectSettingsResolver struct{ *Resolver }
