package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Aliases is the resolver for the aliases field.
func (r *projectSettingsResolver) Aliases(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APIProjectAlias, error) {
	return getAPIAliasesForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// GithubAppAuth is the resolver for the githubAppAuth field.
func (r *projectSettingsResolver) GithubAppAuth(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIGithubAppAuth, error) {
	app, err := githubapp.FindOneGithubAppAuth(utility.FromStringPtr(obj.ProjectRef.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding GitHub app for project '%s': %s", utility.FromStringPtr(obj.ProjectRef.Id), err.Error()))
	}
	// It's valid for a project to not have a GitHub app defined.
	if app == nil {
		return nil, nil
	}
	res := &restModel.APIGithubAppAuth{}
	app = app.RedactPrivateKey()
	res.BuildFromService(*app)
	return res, nil
}

// GithubWebhooksEnabled is the resolver for the githubWebhooksEnabled field.
func (r *projectSettingsResolver) GithubWebhooksEnabled(ctx context.Context, obj *restModel.APIProjectSettings) (bool, error) {
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
func (r *projectSettingsResolver) Subscriptions(ctx context.Context, obj *restModel.APIProjectSettings) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForOwner(ctx, utility.FromStringPtr(obj.ProjectRef.Id), event.OwnerTypeProject)
}

// Vars is the resolver for the vars field.
func (r *projectSettingsResolver) Vars(ctx context.Context, obj *restModel.APIProjectSettings) (*restModel.APIProjectVars, error) {
	return getRedactedAPIVarsForProject(ctx, utility.FromStringPtr(obj.ProjectRef.Id))
}

// ProjectID is the resolver for the projectId field.
func (r *projectSettingsInputResolver) ProjectID(ctx context.Context, obj *restModel.APIProjectSettings, data string) error {
	obj.Id = utility.ToStringPtr(data)
	return nil
}

// ProjectSettings returns ProjectSettingsResolver implementation.
func (r *Resolver) ProjectSettings() ProjectSettingsResolver { return &projectSettingsResolver{r} }

// ProjectSettingsInput returns ProjectSettingsInputResolver implementation.
func (r *Resolver) ProjectSettingsInput() ProjectSettingsInputResolver {
	return &projectSettingsInputResolver{r}
}

type projectSettingsResolver struct{ *Resolver }
type projectSettingsInputResolver struct{ *Resolver }
