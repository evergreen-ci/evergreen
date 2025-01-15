package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Aliases is the resolver for the aliases field.
func (r *repoSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return getAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// GithubAppAuth is the resolver for the githubAppAuth field.
func (r *repoSettingsResolver) GithubAppAuth(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIGithubAppAuth, error) {
	app, err := model.GitHubAppAuthFindOne(utility.FromStringPtr(obj.ProjectRef.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding GitHub app for project '%s': %s", utility.FromStringPtr(obj.ProjectRef.Id), err.Error()))
	}
	// It's valid for a repo to not have a GitHub app defined.
	if app == nil {
		return nil, nil
	}
	res := &restModel.APIGithubAppAuth{}
	app = app.RedactPrivateKey()
	res.BuildFromService(*app)
	return res, nil
}

// GithubWebhooksEnabled is the resolver for the githubWebhooksEnabled field.
func (r *repoSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
	owner := utility.FromStringPtr(obj.ProjectRef.Owner)
	repo := utility.FromStringPtr(obj.ProjectRef.Repo)
	hasApp, err := githubapp.CreateGitHubAppAuth(evergreen.GetEnvironment().Settings()).IsGithubAppInstalledOnRepo(ctx, owner, repo)
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
