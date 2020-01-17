package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

type APIProject struct {
	BatchTime           int                  `json:"batch_time"`
	Branch              *string              `json:"branch_name"`
	DisplayName         *string              `json:"display_name"`
	Enabled             bool                 `json:"enabled"`
	RepotrackerDisabled bool                 `json:"repotracker_disabled"`
	Identifier          *string              `json:"identifier"`
	Owner               *string              `json:"owner_name"`
	Private             bool                 `json:"private"`
	RemotePath          *string              `json:"remote_path"`
	Repo                *string              `json:"repo_name"`
	Tracked             bool                 `json:"tracked"`
	DeactivatePrevious  bool                 `json:"deactivate_previous"`
	Admins              []*string            `json:"admins"`
	Tags                []*string            `json:"tags"`
	TracksPushEvents    bool                 `json:"tracks_push_events"`
	PRTestingEnabled    bool                 `json:"pr_testing_enabled"`
	CommitQueue         APICommitQueueParams `json:"commit_queue"`
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

	cq := APICommitQueueParams{}
	if err := cq.BuildFromService(v.CommitQueue); err != nil {
		return errors.Wrap(err, "can't convert commit queue params")
	}

	apiProject.BatchTime = v.BatchTime
	apiProject.Branch = ToStringPtr(v.Branch)
	apiProject.DisplayName = ToStringPtr(v.DisplayName)
	apiProject.Enabled = v.Enabled
	apiProject.RepotrackerDisabled = v.RepotrackerDisabled
	apiProject.Identifier = ToStringPtr(v.Identifier)
	apiProject.Owner = ToStringPtr(v.Owner)
	apiProject.Private = v.Private
	apiProject.RemotePath = ToStringPtr(v.RemotePath)
	apiProject.Repo = ToStringPtr(v.Repo)
	apiProject.Tracked = v.Tracked
	apiProject.TracksPushEvents = v.TracksPushEvents
	apiProject.PRTestingEnabled = v.PRTestingEnabled
	apiProject.CommitQueue = cq
	apiProject.DeactivatePrevious = v.DeactivatePrevious

	admins := []*string{}
	for _, a := range v.Admins {
		admins = append(admins, ToStringPtr(a))
	}
	apiProject.Admins = admins
	tags := []*string{}
	for _, a := range v.Tags {
		tags = append(tags, ToStringPtr(a))
	}
	apiProject.Tags = tags

	return nil
}

func (apiProject *APIProject) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

type APITriggerDefinition struct {
	Project           *string `json:"project"`
	Level             *string `json:"level"` //build or task
	DefinitionID      *string `json:"definition_id"`
	BuildVariantRegex *string `json:"variant_regex"`
	TaskRegex         *string `json:"task_regex"`
	Status            *string `json:"status"`
	DateCutoff        *int    `json:"date_cutoff"`
	ConfigFile        *string `json:"config_file"`
	GenerateFile      *string `json:"generate_file"`
	Command           *string `json:"command"`
	Alias             *string `json:"alias"`
}

type APICommitQueueParams struct {
	Enabled     bool    `json:"enabled"`
	MergeMethod *string `json:"merge_method"`
	PatchType   *string `json:"patch_type"`
}

func (cqParams *APICommitQueueParams) BuildFromService(h interface{}) error {
	var params model.CommitQueueParams
	switch h.(type) {
	case model.CommitQueueParams:
		params = h.(model.CommitQueueParams)
	case *model.CommitQueueParams:
		params = *h.(*model.CommitQueueParams)
	default:
		return errors.Errorf("Invalid commit queue params of type '%T'", h)
	}

	cqParams.Enabled = params.Enabled
	cqParams.MergeMethod = ToStringPtr(params.MergeMethod)
	cqParams.PatchType = ToStringPtr(params.PatchType)

	return nil
}

func (cqParams *APICommitQueueParams) ToService() (interface{}, error) {
	serviceParams := model.CommitQueueParams{}
	serviceParams.Enabled = cqParams.Enabled
	serviceParams.MergeMethod = FromStringPtr(cqParams.MergeMethod)
	serviceParams.PatchType = FromStringPtr(cqParams.PatchType)

	return serviceParams, nil
}

type APIProjectRef struct {
	Owner                *string              `json:"owner_name"`
	Repo                 *string              `json:"repo_name"`
	Branch               *string              `json:"branch_name"`
	RepoKind             *string              `json:"repo_kind"`
	Enabled              bool                 `json:"enabled"`
	Private              bool                 `json:"private"`
	BatchTime            int                  `json:"batch_time"`
	RemotePath           *string              `json:"remote_path"`
	Identifier           *string              `json:"identifier"`
	DisplayName          *string              `json:"display_name"`
	DeactivatePrevious   bool                 `json:"deactivate_previous"`
	TracksPushEvents     bool                 `json:"tracks_push_events"`
	PRTestingEnabled     bool                 `json:"pr_testing_enabled"`
	DefaultLogger        *string              `json:"default_logger"`
	CommitQueue          APICommitQueueParams `json:"commit_queue"`
	Tracked              bool                 `json:"tracked"`
	PatchingDisabled     bool                 `json:"patching_disabled"`
	RepotrackerDisabled  bool                 `json:"repotracker_disabled"`
	Admins               []*string            `json:"admins"`
	DeleteAdmins         []*string            `json:"delete_admins,omitempty"`
	NotifyOnBuildFailure bool                 `json:"notify_on_failure"`
	Tags                 []*string            `json:"tags"`

	Revision            *string                `json:"revision"`
	Triggers            []APITriggerDefinition `json:"triggers"`
	Aliases             []APIProjectAlias      `json:"aliases"`
	Variables           APIProjectVars         `json:"variables"`
	Subscriptions       []APISubscription      `json:"subscriptions"`
	DeleteSubscriptions []*string              `json:"delete_subscriptions,omitempty"`
}

// ToService returns a service layer ProjectRef using the data from APIProjectRef
func (p *APIProjectRef) ToService() (interface{}, error) {

	commitQueue, err := p.CommitQueue.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "can't convert commit queue params")
	}

	projectRef := model.ProjectRef{
		Owner:                FromStringPtr(p.Owner),
		Repo:                 FromStringPtr(p.Repo),
		Branch:               FromStringPtr(p.Branch),
		RepoKind:             FromStringPtr(p.RepoKind),
		Enabled:              p.Enabled,
		Private:              p.Private,
		BatchTime:            p.BatchTime,
		RemotePath:           FromStringPtr(p.RemotePath),
		Identifier:           FromStringPtr(p.Identifier),
		DisplayName:          FromStringPtr(p.DisplayName),
		DeactivatePrevious:   p.DeactivatePrevious,
		TracksPushEvents:     p.TracksPushEvents,
		DefaultLogger:        FromStringPtr(p.DefaultLogger),
		PRTestingEnabled:     p.PRTestingEnabled,
		CommitQueue:          commitQueue.(model.CommitQueueParams),
		Tracked:              p.Tracked,
		PatchingDisabled:     p.PatchingDisabled,
		RepotrackerDisabled:  p.RepotrackerDisabled,
		NotifyOnBuildFailure: p.NotifyOnBuildFailure,
	}

	// Copy admins
	admins := []string{}
	for _, admin := range p.Admins {
		admins = append(admins, FromStringPtr(admin))
	}
	projectRef.Admins = admins

	// Copy tags
	tags := []string{}
	for _, tag := range p.Tags {
		tags = append(tags, FromStringPtr(tag))
	}
	projectRef.Tags = tags

	// Copy triggers
	triggers := []model.TriggerDefinition{}
	for _, t := range p.Triggers {
		triggers = append(triggers, model.TriggerDefinition{
			Project:           FromStringPtr(t.Project),
			Level:             FromStringPtr(t.Level),
			DefinitionID:      FromStringPtr(t.DefinitionID),
			BuildVariantRegex: FromStringPtr(t.BuildVariantRegex),
			TaskRegex:         FromStringPtr(t.TaskRegex),
			Status:            FromStringPtr(t.Status),
			ConfigFile:        FromStringPtr(t.ConfigFile),
			GenerateFile:      FromStringPtr(t.GenerateFile),
			Command:           FromStringPtr(t.Command),
			Alias:             FromStringPtr(t.Alias),
			DateCutoff:        t.DateCutoff,
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

	cq := APICommitQueueParams{}
	if err := cq.BuildFromService(projectRef.CommitQueue); err != nil {
		return errors.Wrap(err, "can't convert commit queue parameters")
	}

	p.Owner = ToStringPtr(projectRef.Owner)
	p.Repo = ToStringPtr(projectRef.Repo)
	p.Branch = ToStringPtr(projectRef.Branch)
	p.RepoKind = ToStringPtr(projectRef.RepoKind)
	p.Enabled = projectRef.Enabled
	p.Private = projectRef.Private
	p.BatchTime = projectRef.BatchTime
	p.RemotePath = ToStringPtr(projectRef.RemotePath)
	p.Identifier = ToStringPtr(projectRef.Identifier)
	p.DisplayName = ToStringPtr(projectRef.DisplayName)
	p.DeactivatePrevious = projectRef.DeactivatePrevious
	p.TracksPushEvents = projectRef.TracksPushEvents
	p.DefaultLogger = ToStringPtr(projectRef.DefaultLogger)
	p.PRTestingEnabled = projectRef.PRTestingEnabled
	p.CommitQueue = cq
	p.Tracked = projectRef.Tracked
	p.PatchingDisabled = projectRef.PatchingDisabled
	p.RepotrackerDisabled = projectRef.RepotrackerDisabled
	p.NotifyOnBuildFailure = projectRef.NotifyOnBuildFailure

	// Copy admins
	admins := []*string{}
	for _, admin := range projectRef.Admins {
		admins = append(admins, ToStringPtr(admin))
	}
	p.Admins = admins

	// Copy tags
	tags := []*string{}
	for _, tag := range projectRef.Tags {
		tags = append(tags, ToStringPtr(tag))
	}
	p.Tags = tags

	// Copy triggers
	triggers := []APITriggerDefinition{}
	for _, t := range projectRef.Triggers {
		triggers = append(triggers, APITriggerDefinition{
			Project:           ToStringPtr(t.Project),
			Level:             ToStringPtr(t.Level),
			DefinitionID:      ToStringPtr(t.DefinitionID),
			BuildVariantRegex: ToStringPtr(t.BuildVariantRegex),
			TaskRegex:         ToStringPtr(t.TaskRegex),
			Status:            ToStringPtr(t.Status),
			ConfigFile:        ToStringPtr(t.ConfigFile),
			GenerateFile:      ToStringPtr(t.GenerateFile),
			Command:           ToStringPtr(t.Command),
			Alias:             ToStringPtr(t.Alias),
			DateCutoff:        t.DateCutoff,
		})
	}
	p.Triggers = triggers

	return nil
}
