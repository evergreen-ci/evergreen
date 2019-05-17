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
	User      APIString          `json:"user"`
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
	Alias   APIString   `json:"alias"`
	Variant APIString   `json:"variant"`
	Task    APIString   `json:"task"`
	Tags    []APIString `json:"tags,omitempty"`
}

func (e *APIProjectEvent) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.ProjectChangeEventEntry:
		e.Timestamp = v.Timestamp
		data, ok := v.Data.(*model.ProjectChangeEvent)
		if !ok {
			return errors.New("unable to convert event data to project change")
		}

		user := ToAPIString(data.User)
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

	return APIProjectSettings{
		ProjectRef:            apiProjectRef,
		GitHubWebhooksEnabled: settings.GitHubHooksEnabled,
		Vars:                  DbProjectVarsToRestModel(settings.Vars),
		Aliases:               DbProjectAliasesToRestModel(settings.Aliases),
		Subscriptions:         apiSubscriptions,
	}, nil
}

func DbProjectVarsToRestModel(vars model.ProjectVars) APIProjectVars {
	return APIProjectVars{
		Vars:        vars.Vars,
		PrivateVars: vars.PrivateVars,
	}
}

func DbProjectVarsFromRestModel(vars APIProjectVars) model.ProjectVars {
	return model.ProjectVars{
		Vars:        vars.Vars,
		PrivateVars: vars.PrivateVars,
	}
}

func DbProjectAliasesToRestModel(aliases []model.ProjectAlias) []APIProjectAlias {
	result := []APIProjectAlias{}
	for _, alias := range aliases {
		APITags := []APIString{}
		for _, tag := range alias.Tags {
			APITags = append(APITags, ToAPIString(tag))
		}
		apiAlias := APIProjectAlias{
			Alias:   ToAPIString(alias.Alias),
			Variant: ToAPIString(alias.Variant),
			Task:    ToAPIString(alias.Task),
			Tags:    APITags,
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
