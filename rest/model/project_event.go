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
	ProjectRef    APIProjectRef     `json:"proj_ref"`
	Vars          APIProjectVars    `json:"vars"`
	Aliases       []APIProjectAlias `json:"aliases"`
	Subscriptions []APISubscription `json:"subscriptions"`
}

type APIProjectRef struct {
	Owner                APIString   `json:"owner_name"`
	Repo                 APIString   `json:"repo_name"`
	Branch               APIString   `json:"branch_name"`
	Enabled              bool        `json:"enabled"`
	Private              bool        `json:"private"`
	BatchTime            int         `json:"batch_time"`
	RemotePath           APIString   `json:"remote_path"`
	Identifier           APIString   `json:"identifier"`
	DisplayName          APIString   `json:"display_name"`
	DeactivatePrevious   bool        `json:"deactivate_previous"`
	TracksPushEvents     bool        `json:"tracks_push_events"`
	PRTestingEnabled     bool        `json:"pr_testing_enabled"`
	PatchingDisabled     bool        `json:"patching_disabled"`
	Admins               []APIString `json:"admins"`
	NotifyOnBuildFailure bool        `json:"notify_on_failure"`
	GitHubHooksEnabled   bool        `json:"github_hooks_enabled"`
}

type APIProjectVars struct {
	Vars        map[string]string `json:"vars"`
	PrivateVars map[string]bool   `json:"private_vars"`
}

type APIProjectAlias struct {
	Alias   APIString   `json:"alias"`
	Variant APIString   `json:"variant"`
	Task    APIString   `json:"task"`
	Tags    []APIString `json:"tags,omitempty"`
}

func (e *APIProjectEvent) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.ProjectChangeEvent:
		e.Timestamp = v.Timestamp
		data, ok := v.Data.(*model.ProjectChange)
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

func DbProjectSettingsToRestModel(settings model.ProjectSettings) (APIProjectSettings, error) {
	apiSubscriptions, err := DbProjectSubscriptionsToRestModel(settings.Subscriptions)
	return APIProjectSettings{
		ProjectRef:    DbProjectRefToRestModel(settings.ProjectRef, settings.GitHubHooksEnabled),
		Vars:          DbProjectVarsToRestModel(settings.Vars),
		Aliases:       DbProjectAliasesToRestModel(settings.Aliases),
		Subscriptions: apiSubscriptions,
	}, err
}

func DbProjectRefToRestModel(projRef model.ProjectRef, githubHooksEnabled bool) APIProjectRef {
	apiAdmins := []APIString{}
	for _, admin := range projRef.Admins {
		apiAdmins = append(apiAdmins, ToAPIString(admin))
	}

	return APIProjectRef{
		Owner:                ToAPIString(projRef.Owner),
		Repo:                 ToAPIString(projRef.Repo),
		Branch:               ToAPIString(projRef.Branch),
		Enabled:              projRef.Enabled,
		Private:              projRef.Private,
		BatchTime:            projRef.BatchTime,
		RemotePath:           ToAPIString(projRef.RemotePath),
		Identifier:           ToAPIString(projRef.Identifier),
		DisplayName:          ToAPIString(projRef.DisplayName),
		DeactivatePrevious:   projRef.DeactivatePrevious,
		TracksPushEvents:     projRef.TracksPushEvents,
		PRTestingEnabled:     projRef.PRTestingEnabled,
		PatchingDisabled:     projRef.PatchingDisabled,
		Admins:               apiAdmins,
		NotifyOnBuildFailure: projRef.NotifyOnBuildFailure,
		GitHubHooksEnabled:   githubHooksEnabled,
	}
}

func DbProjectVarsToRestModel(vars model.ProjectVars) APIProjectVars {
	return APIProjectVars{
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
