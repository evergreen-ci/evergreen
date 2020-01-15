package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type APIProjectEvent struct {
	Timestamp time.Time          `json:"ts"`
	User      *string          `json:"user"`
	Before    APIProjectSettings `json:"before"`
	After     APIProjectSettings `json:"after"`
}

type APIProjectSettings struct {
	ProjectRef            APIProjectRef     `json:"proj_ref"`
	GitHubWebhooksEnabled bool              `json:"github_webhooks_enabled"`
	Vars                  APIProjectVars    `json:"vars"`
	Aliases               []APIProjectAlias `json:"aliases"`
	Subscriptions         []APISubscription `json:"subscriptions"`
}

type APIProjectVars struct {
	Vars         map[string]string `json:"vars"`
	PrivateVars  map[string]bool   `json:"private_vars"`
	VarsToDelete []string          `json:"vars_to_delete,omitempty"`
}

type APIProjectAlias struct {
	Alias       *string   `json:"alias"`
	Variant     *string   `json:"variant"`
	Task        *string   `json:"task"`
	VariantTags []*string `json:"variant_tags,omitempty"`
	TaskTags    []*string `json:"tags,omitempty"`
	Delete      bool        `json:"delete,omitempty"`
	ID          *string   `json:"_id,omitempty"`
}

func (e *APIProjectEvent) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.ProjectChangeEventEntry:
		e.Timestamp = v.Timestamp
		data, ok := v.Data.(*model.ProjectChangeEvent)
		if !ok {
			return errors.New("unable to convert event data to project change")
		}

		user := ToStringPtr(data.User)
		before, err := DbProjectSettingsToRestModel(data.Before)
		if err != nil {
			return errors.Wrap(err, "unable to convert 'before' changes")
		}
		after, err := DbProjectSettingsToRestModel(data.After)
		if err != nil {
			return errors.Wrap(err, "unable to convert 'after' changes")
		}

		e.User = user
		e.Before = before
		e.After = after
	default:
		return fmt.Errorf("%T is not the correct event type", h)
	}

	return nil
}

func (e *APIProjectEvent) ToService() (interface{}, error) {
	return nil, errors.New("ToService not implemented for APIProjectEvent")
}

func DbProjectSettingsToRestModel(settings model.ProjectSettingsEvent) (APIProjectSettings, error) {
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
		GitHubWebhooksEnabled: settings.GitHubHooksEnabled,
		Vars:                  apiProjectVars,
		Aliases:               DbProjectAliasesToRestModel(settings.Aliases),
		Subscriptions:         apiSubscriptions,
	}, nil
}

func (p *APIProjectVars) ToService() (interface{}, error) {
	privateVars := map[string]bool{}
	// ignore false inputs
	for key, val := range p.PrivateVars {
		if val {
			privateVars[key] = val
		}
	}
	return &model.ProjectVars{
		Vars:        p.Vars,
		PrivateVars: privateVars,
	}, nil
}

func (p *APIProjectVars) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *model.ProjectVars:
		p.PrivateVars = v.PrivateVars
		p.Vars = v.Vars
	default:
		return errors.New("Invalid type of the argument")
	}
	return nil
}

func (a *APIProjectAlias) ToService() (interface{}, error) {
	res := model.ProjectAlias{
		Alias:       FromStringPtr(a.Alias),
		Task:        FromStringPtr(a.Task),
		Variant:     FromStringPtr(a.Variant),
		TaskTags:    FromStringPtrSlice(a.TaskTags),
		VariantTags: FromStringPtrSlice(a.VariantTags),
	}
	if model.IsValidId(FromStringPtr(a.ID)) {
		res.ID = model.NewId(FromStringPtr(a.ID))
	}
	return res, nil
}

func (a *APIProjectAlias) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *model.ProjectAlias:
		APITaskTags := ToStringPtrSlice(v.TaskTags)
		APIVariantTags := ToStringPtrSlice(v.VariantTags)

		a.Alias = ToStringPtr(v.Alias)
		a.Variant = ToStringPtr(v.Variant)
		a.Task = ToStringPtr(v.Task)
		a.VariantTags = APIVariantTags
		a.TaskTags = APITaskTags
		a.ID = ToStringPtr(v.ID.Hex())
	case model.ProjectAlias:
		APITaskTags := ToStringPtrSlice(v.TaskTags)
		APIVariantTags := ToStringPtrSlice(v.VariantTags)

		a.Alias = ToStringPtr(v.Alias)
		a.Variant = ToStringPtr(v.Variant)
		a.Task = ToStringPtr(v.Task)
		a.VariantTags = APIVariantTags
		a.TaskTags = APITaskTags
		a.ID = ToStringPtr(v.ID.Hex())
	default:
		return errors.New("Invalid type of argument")
	}
	return nil
}

func DbProjectAliasesToRestModel(aliases []model.ProjectAlias) []APIProjectAlias {
	result := []APIProjectAlias{}
	for _, alias := range aliases {
		apiAlias := APIProjectAlias{
			Alias:       ToStringPtr(alias.Alias),
			Variant:     ToStringPtr(alias.Variant),
			Task:        ToStringPtr(alias.Task),
			TaskTags:    ToStringPtrSlice(alias.TaskTags),
			VariantTags: ToStringPtrSlice(alias.VariantTags),
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
