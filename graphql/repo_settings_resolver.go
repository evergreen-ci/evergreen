package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Aliases is the resolver for the aliases field.
func (r *repoSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return getAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// GithubWebhooksEnabled is the resolver for the githubWebhooksEnabled field.
func (r *repoSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
	owner := utility.FromStringPtr(obj.ProjectRef.Owner)
	repo := utility.FromStringPtr(obj.ProjectRef.Repo)
	hasApp, err := evergreen.GetEnvironment().Settings().HasGitHubApp(ctx, owner, repo)
	grip.Error(message.WrapError(err, message.Fields{
		"message": "Error verifying GitHub app installation",
		"project": utility.FromStringPtr(obj.ProjectRef.Id),
		"owner":   owner,
		"repo":    repo,
	}))
	return hasApp, nil
}

// Subscriptions is the resolver for the subscriptions field.
func (r *repoSettingsResolver) Subscriptions(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForOwner(ctx, utility.FromStringPtr(obj.ProjectRef.Id), event.OwnerTypeProject)
}

// Vars is the resolver for the vars field.
func (r *repoSettingsResolver) Vars(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	return getRedactedAPIVarsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// RepoID is the resolver for the repoId field.
func (r *repoSettingsInputResolver) RepoID(ctx context.Context, obj *restModel.APIProjectSettings, data string) error {
	obj.Id = utility.ToStringPtr(data)
	return nil
}

// RepoSettings returns RepoSettingsResolver implementation.
func (r *Resolver) RepoSettings() RepoSettingsResolver { return &repoSettingsResolver{r} }

// RepoSettingsInput returns RepoSettingsInputResolver implementation.
func (r *Resolver) RepoSettingsInput() RepoSettingsInputResolver {
	return &repoSettingsInputResolver{r}
}

type repoSettingsResolver struct{ *Resolver }
type repoSettingsInputResolver struct{ *Resolver }
