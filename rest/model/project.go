package model

import (
	"errors"

	"github.com/evergreen-ci/evergreen/model"
)

type APIProject struct {
	BatchTime              int         `json:"batch_time"`
	Branch                 APIString   `json:"branch_name"`
	DisplayName            APIString   `json:"display_name"`
	Enabled                bool        `json:"enabled"`
	Identifier             APIString   `json:"identifier"`
	Owner                  APIString   `json:"owner_name"`
	Private                bool        `json:"private"`
	RemotePath             APIString   `json:"remote_path"`
	Repo                   APIString   `json:"repo_name"`
	Tracked                bool        `json:"tracked"`
	DeactivatePrevious     bool        `json:"deactivate_previous"`
	Admins                 []APIString `json:"admins"`
	TracksPushEvents       bool        `json:"tracks_push_events"`
	PRTestingEnabled       bool        `json:"pr_testing_enabled"`
	CommitQueueEnabled     bool        `json:"commitq_enabled"`
	CommitQueueFile        APIString   `json:"commitq_file"`
	CommitQueueMergeMethod APIString   `json:"commitq_merge_method"`
}

func (apiProject *APIProject) BuildFromService(p interface{}) error {
	var v model.ProjectRef

	switch p.(type) {
	case model.ProjectRef:
		v = p.(model.ProjectRef)
	case *model.ProjectRef:
		v = *p.(*model.ProjectRef)
	default:
		return errors.New("incorrect type when fetching converting project type")
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
	apiProject.CommitQueueEnabled = v.CommitQueueEnabled
	apiProject.CommitQueueFile = ToAPIString(v.CommitQueueConfigFile)
	apiProject.CommitQueueMergeMethod = ToAPIString(v.CommitQueueMergeMethod)
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
	BuildVariantRegex APIString `json:"variant_regex"`
	TaskRegex         APIString `json:"task_regex"`
	Status            APIString `json:"status"`
	ConfigFile        APIString `json:"config_file"`
	Command           APIString `json:"command"`
}

type APIProjectRef struct {
	Owner                  APIString   `json:"owner_name"`
	Repo                   APIString   `json:"repo_name"`
	Branch                 APIString   `json:"branch_name"`
	RepoKind               APIString   `json:"repo_kind"`
	Enabled                bool        `json:"enabled"`
	Private                bool        `json:"private"`
	BatchTime              int         `json:"batch_time"`
	RemotePath             APIString   `json:"remote_path"`
	Identifier             APIString   `json:"identifier"`
	DisplayName            APIString   `json:"display_name"`
	LocalConfig            APIString   `json:"local_config"`
	DeactivatePrevious     bool        `json:"deactivate_previous"`
	TracksPushEvents       bool        `json:"tracks_push_events"`
	PRTestingEnabled       bool        `json:"pr_testing_enabled"`
	CommitQueueEnabled     bool        `json:"commitq_enabled"`
	CommitQueueFile        APIString   `json:"commitq_file"`
	CommitQueueMergeMethod APIString   `json:"commitq_merge_method"`
	Tracked                bool        `json:"tracked"`
	PatchingDisabled       bool        `json:"patching_disabled"`
	Admins                 []APIString `json:"admins"`
	NotifyOnBuildFailure   bool        `json:"notify_on_failure"`

	Triggers []APITriggerDefinition `json:"triggers"`
}

// ToService returns a service layer ProjectRef using the data from APIProjectRef
func (p *APIProjectRef) ToService() (interface{}, error) {
	projectRef := model.ProjectRef{
		Owner:                  FromAPIString(p.Owner),
		Repo:                   FromAPIString(p.Repo),
		Branch:                 FromAPIString(p.Branch),
		RepoKind:               FromAPIString(p.RepoKind),
		Enabled:                p.Enabled,
		Private:                p.Private,
		BatchTime:              p.BatchTime,
		RemotePath:             FromAPIString(p.RemotePath),
		Identifier:             FromAPIString(p.Identifier),
		DisplayName:            FromAPIString(p.DisplayName),
		LocalConfig:            FromAPIString(p.LocalConfig),
		DeactivatePrevious:     p.DeactivatePrevious,
		TracksPushEvents:       p.TracksPushEvents,
		PRTestingEnabled:       p.PRTestingEnabled,
		CommitQueueEnabled:     p.CommitQueueEnabled,
		CommitQueueConfigFile:  FromAPIString(p.CommitQueueFile),
		CommitQueueMergeMethod: FromAPIString(p.CommitQueueMergeMethod),
		Tracked:                p.Tracked,
		PatchingDisabled:       p.PatchingDisabled,
		NotifyOnBuildFailure:   p.NotifyOnBuildFailure,
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

	return &projectRef, nil
}

func (p *APIProjectRef) BuildFromService(v interface{}) error {
	var projectRef model.ProjectRef

	switch v.(type) {
	case model.ProjectRef:
		projectRef = v.(model.ProjectRef)
	case *model.ProjectRef:
		projectRef = *v.(*model.ProjectRef)
	default:
		return errors.New("Invalid type of the argument")
	}

	p.Owner = ToAPIString(projectRef.Owner)
	p.Repo = ToAPIString(projectRef.Repo)
	p.Branch = ToAPIString(projectRef.Branch)
	p.RepoKind = ToAPIString(projectRef.RepoKind)
	p.Enabled = projectRef.Enabled
	p.Private = projectRef.Private
	p.BatchTime = projectRef.BatchTime
	p.RemotePath = ToAPIString(projectRef.RemotePath)
	p.Identifier = ToAPIString(projectRef.Identifier)
	p.DisplayName = ToAPIString(projectRef.DisplayName)
	p.LocalConfig = ToAPIString(projectRef.LocalConfig)
	p.DeactivatePrevious = projectRef.DeactivatePrevious
	p.TracksPushEvents = projectRef.TracksPushEvents
	p.PRTestingEnabled = projectRef.PRTestingEnabled
	p.CommitQueueEnabled = projectRef.CommitQueueEnabled
	p.CommitQueueFile = ToAPIString(projectRef.CommitQueueConfigFile)
	p.CommitQueueMergeMethod = ToAPIString(projectRef.CommitQueueMergeMethod)
	p.Tracked = projectRef.Tracked
	p.PatchingDisabled = projectRef.PatchingDisabled
	p.NotifyOnBuildFailure = projectRef.NotifyOnBuildFailure

	// Copy admins
	admins := []APIString{}
	for _, admin := range projectRef.Admins {
		admins = append(admins, ToAPIString(admin))
	}
	p.Admins = admins

	// Copy triggers
	triggers := []APITriggerDefinition{}
	for _, t := range projectRef.Triggers {
		triggers = append(triggers, APITriggerDefinition{
			Project:           ToAPIString(t.Project),
			Level:             ToAPIString(t.Level),
			DefinitionID:      ToAPIString(t.DefinitionID),
			BuildVariantRegex: ToAPIString(t.BuildVariantRegex),
			TaskRegex:         ToAPIString(t.TaskRegex),
			Status:            ToAPIString(t.Status),
			ConfigFile:        ToAPIString(t.ConfigFile),
			Command:           ToAPIString(t.Command),
		})
	}
	p.Triggers = triggers

	return nil
}
