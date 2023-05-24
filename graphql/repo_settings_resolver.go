package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Aliases is the resolver for the aliases field.
func (r *repoSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return getAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// GithubWebhooksEnabled is the resolver for the githubWebhooksEnabled field.
func (r *repoSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
	hook, err := model.FindGithubHook(utility.FromStringPtr(obj.ProjectRef.Owner), utility.FromStringPtr(obj.ProjectRef.Repo))
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Database error finding github hook for project '%s': %s", *obj.ProjectRef.Id, err.Error()))
	}
	return hook != nil, nil
}

// Subscriptions is the resolver for the subscriptions field.
func (r *repoSettingsResolver) Subscriptions(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForOwner(ctx, utility.FromStringPtr(obj.ProjectRef.Id), event.OwnerTypeProject)
}

// Vars is the resolver for the vars field.
func (r *repoSettingsResolver) Vars(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	return getRedactedAPIVarsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// RepoSettings returns RepoSettingsResolver implementation.
func (r *Resolver) RepoSettings() RepoSettingsResolver { return &repoSettingsResolver{r} }

type repoSettingsResolver struct{ *Resolver }
