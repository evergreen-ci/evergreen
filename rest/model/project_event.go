package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
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
	ProjectRef            APIProjectRef     `json:"proj_ref"`
	GithubWebhooksEnabled bool              `json:"github_webhooks_enabled"`
	Vars                  APIProjectVars    `json:"vars"`
	Aliases               []APIProjectAlias `json:"aliases"`
	Subscriptions         []APISubscription `json:"subscriptions"`
}

type APIProjectSettings struct {
	ProjectRef            APIProjectRef     `json:"proj_ref"`
	GithubWebhooksEnabled bool              `json:"github_webhooks_enabled"`
	Vars                  APIProjectVars    `json:"vars"`
	Aliases               []APIProjectAlias `json:"aliases"`
	Subscriptions         []APISubscription `json:"subscriptions"`
}

type APIProjectVars struct {
	Vars          map[string]string `json:"vars"`
	PrivateVars   map[string]bool   `json:"private_vars"`
	AdminOnlyVars map[string]bool   `json:"admin_only_vars"`
	VarsToDelete  []string          `json:"vars_to_delete,omitempty"`

	// to use for the UI
	PrivateVarsList   []string `json:"-"`
	AdminOnlyVarsList []string `json:"-"`
}

type APIProjectAlias struct {
	Alias       *string         `json:"alias"`
	GitTag      *string         `json:"git_tag"`
	Variant     *string         `json:"variant"`
	Description *string         `json:"description"`
	Task        *string         `json:"task"`
	RemotePath  *string         `json:"remote_path"`
	VariantTags []*string       `json:"variant_tags,omitempty"`
	TaskTags    []*string       `json:"tags,omitempty"`
	Delete      bool            `json:"delete,omitempty"`
	ID          *string         `json:"_id,omitempty"`
	Parameters  []*APIParameter `json:"parameters,omitempty"`
}

func (e *APIProjectEvent) BuildFromService(entry model.ProjectChangeEventEntry) error {
	e.Timestamp = ToTimePtr(entry.Timestamp)
	data, ok := entry.Data.(*model.ProjectChangeEvent)
	if !ok {
		return errors.Errorf("programmatic error: expected project change event but got type %T", entry.Data)
	}

	user := utility.ToStringPtr(data.User)
	before, err := DbProjectSettingsToRestModel(data.Before.ProjectSettings)
	if err != nil {
		return errors.Wrap(err, "converting 'before' project settings to API model")
	}
	after, err := DbProjectSettingsToRestModel(data.After.ProjectSettings)
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

	return APIProjectSettings{
		ProjectRef:            apiProjectRef,
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
