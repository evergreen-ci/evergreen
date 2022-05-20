package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
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
	Alias       *string   `json:"alias"`
	GitTag      *string   `json:"git_tag"`
	Variant     *string   `json:"variant"`
	Task        *string   `json:"task"`
	RemotePath  *string   `json:"remote_path"`
	VariantTags []*string `json:"variant_tags,omitempty"`
	TaskTags    []*string `json:"tags,omitempty"`
	Delete      bool      `json:"delete,omitempty"`
	ID          *string   `json:"_id,omitempty"`
}

func (e *APIProjectEvent) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.ProjectChangeEventEntry:
		e.Timestamp = ToTimePtr(v.Timestamp)
		data, ok := v.Data.(*model.ProjectChangeEvent)
		if !ok {
			return errors.Errorf("programmatic error: expected project change event but got type %T", v.Data)
		}

		user := utility.ToStringPtr(data.User)
		before, err := DbProjectSettingsToRestModel(data.Before)
		if err != nil {
			return errors.Wrap(err, "converting 'before' project settings to API model")
		}
		after, err := DbProjectSettingsToRestModel(data.After)
		if err != nil {
			return errors.Wrap(err, "converting 'after' project settings to API model")
		}

		e.User = user
		e.Before = APIProjectEventSettings{
			ProjectRef:            before.ProjectRef,
			GithubWebhooksEnabled: before.GithubWebhooksEnabled,
			Vars:                  before.Vars,
			Aliases:               before.Aliases,
			Subscriptions:         before.Subscriptions,
		}
		e.After = APIProjectEventSettings{
			ProjectRef:            after.ProjectRef,
			GithubWebhooksEnabled: after.GithubWebhooksEnabled,
			Vars:                  after.Vars,
			Aliases:               after.Aliases,
			Subscriptions:         after.Subscriptions,
		}
	default:
		return errors.Errorf("programmatic error: expected project change event entry but got type %T", h)
	}

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
	if err := apiProjectVars.BuildFromService(&settings.Vars); err != nil {
		return APIProjectSettings{}, err
	}

	return APIProjectSettings{
		ProjectRef:            apiProjectRef,
		GithubWebhooksEnabled: settings.GithubHooksEnabled,
		Vars:                  apiProjectVars,
		Aliases:               dbProjectAliasesToRestModel(settings.Aliases),
		Subscriptions:         apiSubscriptions,
	}, nil
}

func (p *APIProjectVars) ToService() (interface{}, error) {
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
	}, nil
}

func (p *APIProjectVars) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *model.ProjectVars:
		p.PrivateVars = v.PrivateVars
		p.Vars = v.Vars
		p.AdminOnlyVars = v.AdminOnlyVars
	default:
		return errors.Errorf("programmatic error: expected project variables but got type %T", h)
	}
	return nil
}

func (a *APIProjectAlias) ToService() (interface{}, error) {
	res := model.ProjectAlias{
		Alias:       utility.FromStringPtr(a.Alias),
		Task:        utility.FromStringPtr(a.Task),
		Variant:     utility.FromStringPtr(a.Variant),
		GitTag:      utility.FromStringPtr(a.GitTag),
		RemotePath:  utility.FromStringPtr(a.RemotePath),
		TaskTags:    utility.FromStringPtrSlice(a.TaskTags),
		VariantTags: utility.FromStringPtrSlice(a.VariantTags),
	}
	if model.IsValidId(utility.FromStringPtr(a.ID)) {
		res.ID = model.NewId(utility.FromStringPtr(a.ID))
	}
	return res, nil
}

func (a *APIProjectAlias) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *model.ProjectAlias:
		APITaskTags := utility.ToStringPtrSlice(v.TaskTags)
		APIVariantTags := utility.ToStringPtrSlice(v.VariantTags)

		a.Alias = utility.ToStringPtr(v.Alias)
		a.Variant = utility.ToStringPtr(v.Variant)
		a.GitTag = utility.ToStringPtr(v.GitTag)
		a.RemotePath = utility.ToStringPtr(v.RemotePath)
		a.Task = utility.ToStringPtr(v.Task)
		a.VariantTags = APIVariantTags
		a.TaskTags = APITaskTags
		a.ID = utility.ToStringPtr(v.ID.Hex())
	case model.ProjectAlias:
		APITaskTags := utility.ToStringPtrSlice(v.TaskTags)
		APIVariantTags := utility.ToStringPtrSlice(v.VariantTags)

		a.Alias = utility.ToStringPtr(v.Alias)
		a.Variant = utility.ToStringPtr(v.Variant)
		a.GitTag = utility.ToStringPtr(v.GitTag)
		a.RemotePath = utility.ToStringPtr(v.RemotePath)
		a.Task = utility.ToStringPtr(v.Task)
		a.VariantTags = APIVariantTags
		a.TaskTags = APITaskTags
		a.ID = utility.ToStringPtr(v.ID.Hex())
	default:
		return errors.Errorf("programmatic error: expected project alias but got type %T", h)
	}
	return nil
}

func dbProjectAliasesToRestModel(aliases []model.ProjectAlias) []APIProjectAlias {
	result := []APIProjectAlias{}
	for _, alias := range aliases {
		apiAlias := APIProjectAlias{
			ID:          utility.ToStringPtr(alias.ID.String()),
			Alias:       utility.ToStringPtr(alias.Alias),
			Variant:     utility.ToStringPtr(alias.Variant),
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
