package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type APIProjectEvent struct {
	Timestamp *time.Time              `json:"ts"`
	User      *string                 `json:"user"`
	Before    APIProjectEventSettings `json:"before"`
	After     APIProjectEventSettings `json:"after"`
}

// take this from the original place instead of redefinning it here
type APIProjectEventSettings struct {
	Id                    *string           `json:"id"`
	ProjectRef            APIProjectRef     `json:"proj_ref"`
	GithubAppAuth         APIGithubAppAuth  `json:"github_app_auth"`
	GithubWebhooksEnabled bool              `json:"github_webhooks_enabled"`
	Vars                  APIProjectVars    `json:"vars"`
	Aliases               []APIProjectAlias `json:"aliases"`
	Subscriptions         []APISubscription `json:"subscriptions"`
}

type APIProjectSettings struct {
	Id                    *string           `json:"id"`
	ProjectRef            APIProjectRef     `json:"proj_ref"`
	GithubAppAuth         APIGithubAppAuth  `json:"github_app_auth"`
	GithubWebhooksEnabled bool              `json:"github_webhooks_enabled"`
	Vars                  APIProjectVars    `json:"vars"`
	Aliases               []APIProjectAlias `json:"aliases"`
	Subscriptions         []APISubscription `json:"subscriptions"`
}

type APIGithubAppAuth struct {
	AppID      int     `json:"app_id"`
	PrivateKey *string `json:"private_key"`
}

type APIProjectVars struct {
	// Regular project variable names and their values.
	Vars map[string]string `json:"vars"`
	// Private variable names.
	PrivateVars map[string]bool `json:"private_vars"`
	// Admin-only variable names.
	AdminOnlyVars map[string]bool `json:"admin_only_vars"`
	// Names of project variables to delete.
	VarsToDelete []string `json:"vars_to_delete,omitempty"`

	// to use for the UI
	PrivateVarsList   []string `json:"-"`
	AdminOnlyVarsList []string `json:"-"`
}

type APIProjectAlias struct {
	// Name of the alias.
	Alias *string `json:"alias"`
	// Regex for matching git tags to run git tag versions.
	GitTag *string `json:"git_tag"`
	// Regex for build variants to match.
	Variant *string `json:"variant"`
	// Human-friendly description for the alias.
	Description *string `json:"description"`
	// Regex for tasks to match.
	Task *string `json:"task"`
	// Path to project config file to use.
	RemotePath *string `json:"remote_path"`
	// Build variant tags selectors to match.
	VariantTags []*string `json:"variant_tags,omitempty"`
	// Task tag selectors to match.
	TaskTags []*string `json:"tags,omitempty"`
	// If set, deletes the project alias by name.
	Delete bool `json:"delete,omitempty"`
	// Identifier for the project alias.
	ID *string `json:"_id,omitempty"`
	// List of allowed parameters to the alias.
	Parameters []*APIParameter `json:"parameters,omitempty"`
}

func (e *APIProjectEvent) BuildFromService(entry model.ProjectChangeEventEntry) error {
	e.Timestamp = ToTimePtr(entry.Timestamp)
	data, ok := entry.Data.(*model.ProjectChangeEvent)
	if !ok {
		return errors.Errorf("programmatic error: expected project change event but got type %T", entry.Data)
	}

	user := utility.ToStringPtr(data.User)
	before, err := DbProjectSettingsToRestModel(model.NewProjectSettingsFromEvent(data.Before))
	if err != nil {
		return errors.Wrap(err, "converting 'before' project settings to API model")
	}
	after, err := DbProjectSettingsToRestModel(model.NewProjectSettingsFromEvent(data.After))
	if err != nil {
		return errors.Wrap(err, "converting 'after' project settings to API model")
	}

	e.User = user
	e.Before = APIProjectEventSettings(before)
	e.After = APIProjectEventSettings(after)
	return nil
}

func (e *APIProjectEvent) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for APIProjectEvent")
}

func DbProjectSettingsToRestModel(settings model.ProjectSettings) (APIProjectSettings, error) {
	apiProjectRef := APIProjectRef{}
	if err := apiProjectRef.BuildFromService(settings.ProjectRef); err != nil {
		return APIProjectSettings{}, err
	}

	apiSubscriptions, err := DbProjectSubscriptionsToRestModel(settings.Subscriptions)
	if err != nil {
		return APIProjectSettings{}, err
	}

	apiProjectVars := APIProjectVars{}
	apiProjectVars.BuildFromService(settings.Vars)

	githubAuthApp := APIGithubAppAuth{}
	githubAuthApp.BuildFromService(settings.GitHubAppAuth)

	return APIProjectSettings{
		ProjectRef:            apiProjectRef,
		GithubAppAuth:         githubAuthApp,
		GithubWebhooksEnabled: settings.GithubHooksEnabled,
		Vars:                  apiProjectVars,
		Aliases:               dbProjectAliasesToRestModel(settings.Aliases),
		Subscriptions:         apiSubscriptions,
	}, nil
}

func (p *APIProjectVars) ToService() *model.ProjectVars {
	privateVars := map[string]bool{}
	adminOnlyVars := map[string]bool{}
	// ignore false inputs
	for key, val := range p.PrivateVars {
		if val {
			privateVars[key] = val
		}
	}
	for key, val := range p.AdminOnlyVars {
		if val {
			adminOnlyVars[key] = val
		}
	}

	// handle UI list
	for _, each := range p.PrivateVarsList {
		privateVars[each] = true
	}
	for _, each := range p.AdminOnlyVarsList {
		adminOnlyVars[each] = true
	}
	return &model.ProjectVars{
		Vars:          p.Vars,
		AdminOnlyVars: adminOnlyVars,
		PrivateVars:   privateVars,
	}
}

func (p *APIProjectVars) BuildFromService(v model.ProjectVars) {
	p.PrivateVars = v.PrivateVars
	p.Vars = v.Vars
	p.AdminOnlyVars = v.AdminOnlyVars
}

func (a *APIProjectAlias) ToService() model.ProjectAlias {
	res := model.ProjectAlias{
		Alias:       utility.FromStringPtr(a.Alias),
		Task:        utility.FromStringPtr(a.Task),
		Variant:     utility.FromStringPtr(a.Variant),
		Description: utility.FromStringPtr(a.Description),
		GitTag:      utility.FromStringPtr(a.GitTag),
		RemotePath:  utility.FromStringPtr(a.RemotePath),
		TaskTags:    utility.FromStringPtrSlice(a.TaskTags),
		VariantTags: utility.FromStringPtrSlice(a.VariantTags),
	}
	res.Parameters = []patch.Parameter{}
	for _, param := range a.Parameters {
		res.Parameters = append(res.Parameters, param.ToService())
	}

	if model.IsValidId(utility.FromStringPtr(a.ID)) {
		res.ID = model.NewId(utility.FromStringPtr(a.ID))
	}
	return res
}

func (a *APIProjectAlias) BuildFromService(in model.ProjectAlias) {
	APITaskTags := utility.ToStringPtrSlice(in.TaskTags)
	APIVariantTags := utility.ToStringPtrSlice(in.VariantTags)

	a.Alias = utility.ToStringPtr(in.Alias)
	a.Variant = utility.ToStringPtr(in.Variant)
	a.Description = utility.ToStringPtr(in.Description)
	a.GitTag = utility.ToStringPtr(in.GitTag)
	a.RemotePath = utility.ToStringPtr(in.RemotePath)
	a.Task = utility.ToStringPtr(in.Task)
	a.VariantTags = APIVariantTags
	a.TaskTags = APITaskTags
	a.ID = utility.ToStringPtr(in.ID.Hex())
	APIParameters := []*APIParameter{}
	for _, param := range in.Parameters {
		APIParam := &APIParameter{}
		APIParam.BuildFromService(&param)
		APIParameters = append(APIParameters, APIParam)
	}
	a.Parameters = APIParameters
}

func dbProjectAliasesToRestModel(aliases []model.ProjectAlias) []APIProjectAlias {
	result := []APIProjectAlias{}
	for _, alias := range aliases {
		apiAlias := APIProjectAlias{
			ID:          utility.ToStringPtr(alias.ID.String()),
			Alias:       utility.ToStringPtr(alias.Alias),
			Variant:     utility.ToStringPtr(alias.Variant),
			Description: utility.ToStringPtr(alias.Description),
			Task:        utility.ToStringPtr(alias.Task),
			RemotePath:  utility.ToStringPtr(alias.RemotePath),
			GitTag:      utility.ToStringPtr(alias.GitTag),
			TaskTags:    utility.ToStringPtrSlice(alias.TaskTags),
			VariantTags: utility.ToStringPtrSlice(alias.VariantTags),
		}
		result = append(result, apiAlias)
	}

	return result
}

func DbProjectSubscriptionsToRestModel(subscriptions []event.Subscription) ([]APISubscription, error) {
	catcher := grip.NewBasicCatcher()
	apiSubscriptions := []APISubscription{}
	for _, subscription := range subscriptions {
		apiSubscription := APISubscription{}
		if err := apiSubscription.BuildFromService(subscription); err != nil {
			catcher.Add(err)
			continue
		}
		apiSubscriptions = append(apiSubscriptions, apiSubscription)
	}

	return apiSubscriptions, catcher.Resolve()
}

// IsPrivate returns true if the given key is a private variable.
func (vars *APIProjectVars) IsPrivate(key string) bool {
	if vars.PrivateVars[key] {
		return true
	}
	return utility.StringSliceContains(vars.PrivateVarsList, key)
}

func (g *APIGithubAppAuth) ToService() *githubapp.GithubAppAuth {
	return &githubapp.GithubAppAuth{
		AppID:      int64(g.AppID),
		PrivateKey: []byte(utility.FromStringPtr(g.PrivateKey)),
	}
}

func (g *APIGithubAppAuth) BuildFromService(v githubapp.GithubAppAuth) {
	g.AppID = int(v.AppID)
	g.PrivateKey = utility.ToStringPtr(string(v.PrivateKey))
}
