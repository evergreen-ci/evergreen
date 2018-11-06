package model

import (
	"errors"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
)

type APIProject struct {
	BatchTime          int         `json:"batch_time"`
	Branch             APIString   `json:"branch_name"`
	DisplayName        APIString   `json:"display_name"`
	Enabled            bool        `json:"enabled"`
	Identifier         APIString   `json:"identifier"`
	Owner              APIString   `json:"owner_name"`
	Private            bool        `json:"private"`
	RemotePath         APIString   `json:"remote_path"`
	Repo               APIString   `json:"repo_name"`
	Tracked            bool        `json:"tracked"`
	DeactivatePrevious bool        `json:"deactivate_previous"`
	Admins             []APIString `json:"admins"`
	TracksPushEvents   bool        `json:"tracks_push_events"`
	PRTestingEnabled   bool        `json:"pr_testing_enabled"`
}

func (apiProject *APIProject) BuildFromService(p interface{}) error {
	v, ok := p.(model.ProjectRef)
	if !ok {
		return fmt.Errorf(fmt.Sprintf("incorrect type when fetching converting project type %T", p))
	}
	apiProject.BatchTime = v.BatchTime
	apiProject.Branch = ToAPIString(v.Branch)
	apiProject.DisplayName = ToAPIString(v.DisplayName)
	apiProject.Enabled = v.Enabled
	apiProject.Identifier = ToAPIString(v.Identifier)
	apiProject.Owner = ToAPIString(v.Owner)
	apiProject.Private = v.Private
	apiProject.RemotePath = ToAPIString(v.RemotePath)
	apiProject.Repo = ToAPIString(v.Repo)
	apiProject.Tracked = v.Tracked
	apiProject.TracksPushEvents = v.TracksPushEvents
	apiProject.PRTestingEnabled = v.PRTestingEnabled
	apiProject.DeactivatePrevious = v.DeactivatePrevious

	admins := []APIString{}
	for _, a := range v.Admins {
		admins = append(admins, ToAPIString(a))
	}
	apiProject.Admins = admins

	return nil
}

func (apiProject *APIProject) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

type APITriggerDefinition struct {
	Project           APIString `json:"project"`
	Level             APIString `json:"level"` //build or task
	DefinitionID      APIString `json:"definition_id"`
	BuildVariantRegex APIString `json:"variant_regex,omitempty"`
	TaskRegex         APIString `json:"task_regex,omitempty"`
	Status            APIString `json:"status,omitempty"`
	ConfigFile        APIString `json:"config_file,omitempty"`
	Command           APIString `json:"command,omitempty"`
}

type APIProjectRef struct {
	Owner                APIString   `json:"owner_name"`
	Repo                 APIString   `json:"repo_name"`
	Branch               APIString   `json:"branch_name"`
	RepoKind             APIString   `json:"repo_kind"`
	Enabled              bool        `json:"enabled"`
	Private              bool        `json:"private"`
	BatchTime            int         `json:"batch_time"`
	RemotePath           APIString   `json:"remote_path"th"`
	Identifier           APIString   `json:"identifier"`
	DisplayName          APIString   `json:"display_name"`
	LocalConfig          APIString   `json:"local_config"`
	DeactivatePrevious   bool        `json:"deactivate_previous"`
	TracksPushEvents     bool        `json:"tracks_push_events"`
	PRTestingEnabled     bool        `json:"pr_testing_enabled"`
	Tracked              bool        `json:"tracked"`
	PatchingDisabled     bool        `json:"patching_disabled"`
	Admins               []APIString `json:"admins"`
	NotifyOnBuildFailure bool        `json:"notify_on_failure"`

	Triggers []APITriggerDefinition `json:"triggers,omitempty"`
}

func (apiProject *APIProjectRef) BuildFromService(p interface{}) error {
	return errors.New("Not implemented")
}

func (p *APIProjectRef) ToService() (model.ProjectRef, error) {
	projectRef := model.ProjectRef{
		Owner:                FromAPIString(p.Owner),
		Repo:                 FromAPIString(p.Repo),
		Branch:               FromAPIString(p.Branch),
		RepoKind:             FromAPIString(p.RepoKind),
		Enabled:              p.Enabled,
		Private:              p.Private,
		BatchTime:            p.BatchTime,
		RemotePath:           FromAPIString(p.RemotePath),
		Identifier:           FromAPIString(p.Identifier),
		DisplayName:          FromAPIString(p.DisplayName),
		LocalConfig:          FromAPIString(p.LocalConfig),
		DeactivatePrevious:   p.DeactivatePrevious,
		TracksPushEvents:     p.TracksPushEvents,
		PRTestingEnabled:     p.PRTestingEnabled,
		Tracked:              p.Tracked,
		PatchingDisabled:     p.PatchingDisabled,
		NotifyOnBuildFailure: p.NotifyOnBuildFailure,
	}

	// Copy admins
	admins := []string{}
	for _, admin := range p.Admins {
		admins = append(admins, FromAPIString(admin))
	}
	projectRef.Admins = admins

	// Copy triggers
	triggers := []model.TriggerDefinition{}
	for _, t := range p.Triggers {
		triggers = append(triggers, model.TriggerDefinition{
			Project:           FromAPIString(t.Project),
			Level:             FromAPIString(t.Level),
			DefinitionID:      FromAPIString(t.DefinitionID),
			BuildVariantRegex: FromAPIString(t.BuildVariantRegex),
			TaskRegex:         FromAPIString(t.TaskRegex),
			Status:            FromAPIString(t.Status),
			ConfigFile:        FromAPIString(t.ConfigFile),
			Command:           FromAPIString(t.Command),
		})
	}
	projectRef.Triggers = triggers

	return projectRef, nil
}
