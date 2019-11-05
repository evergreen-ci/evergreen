package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

type APIProject struct {
	BatchTime           int                  `json:"batch_time"`
	Branch              APIString            `json:"branch_name"`
	DisplayName         APIString            `json:"display_name"`
	Enabled             bool                 `json:"enabled"`
	RepotrackerDisabled bool                 `json:"repotracker_disabled"`
	Identifier          APIString            `json:"identifier"`
	Owner               APIString            `json:"owner_name"`
	Private             bool                 `json:"private"`
	RemotePath          APIString            `json:"remote_path"`
	Repo                APIString            `json:"repo_name"`
	Tracked             bool                 `json:"tracked"`
	DeactivatePrevious  bool                 `json:"deactivate_previous"`
	Admins              []APIString          `json:"admins"`
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
	apiProject.Branch = ToAPIString(v.Branch)
	apiProject.DisplayName = ToAPIString(v.DisplayName)
	apiProject.Enabled = v.Enabled
	apiProject.RepotrackerDisabled = v.RepotrackerDisabled
	apiProject.Identifier = ToAPIString(v.Identifier)
	apiProject.Owner = ToAPIString(v.Owner)
	apiProject.Private = v.Private
	apiProject.RemotePath = ToAPIString(v.RemotePath)
	apiProject.Repo = ToAPIString(v.Repo)
	apiProject.Tracked = v.Tracked
	apiProject.TracksPushEvents = v.TracksPushEvents
	apiProject.PRTestingEnabled = v.PRTestingEnabled
	apiProject.CommitQueue = cq
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
	DateCutoff        *int      `json:"date_cutoff"`
	ConfigFile        APIString `json:"config_file"`
	GenerateFile      APIString `json:"generate_file"`
	Command           APIString `json:"command"`
	Alias             APIString `json:"alias"`
}

type APICommitQueueParams struct {
	Enabled     bool      `json:"enabled"`
	MergeMethod APIString `json:"merge_method"`
	PatchType   APIString `json:"patch_type"`
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
	cqParams.MergeMethod = ToAPIString(params.MergeMethod)
	cqParams.PatchType = ToAPIString(params.PatchType)

	return nil
}

func (cqParams *APICommitQueueParams) ToService() (interface{}, error) {
	serviceParams := model.CommitQueueParams{}
	serviceParams.Enabled = cqParams.Enabled
	serviceParams.MergeMethod = FromAPIString(cqParams.MergeMethod)
	serviceParams.PatchType = FromAPIString(cqParams.PatchType)

	return serviceParams, nil
}

type APIProjectRef struct {
	Owner                APIString            `json:"owner_name"`
	Repo                 APIString            `json:"repo_name"`
	Branch               APIString            `json:"branch_name"`
	RepoKind             APIString            `json:"repo_kind"`
	Enabled              bool                 `json:"enabled"`
	Private              bool                 `json:"private"`
	BatchTime            int                  `json:"batch_time"`
	RemotePath           APIString            `json:"remote_path"`
	Identifier           APIString            `json:"identifier"`
	DisplayName          APIString            `json:"display_name"`
	DeactivatePrevious   bool                 `json:"deactivate_previous"`
	TracksPushEvents     bool                 `json:"tracks_push_events"`
	PRTestingEnabled     bool                 `json:"pr_testing_enabled"`
	CommitQueue          APICommitQueueParams `json:"commit_queue"`
	Tracked              bool                 `json:"tracked"`
	PatchingDisabled     bool                 `json:"patching_disabled"`
	Admins               []APIString          `json:"admins"`
	DeleteAdmins         []APIString          `json:"delete_admins,omitempty"`
	NotifyOnBuildFailure bool                 `json:"notify_on_failure"`

	Revision            APIString              `json:"revision"`
	Triggers            []APITriggerDefinition `json:"triggers"`
	Aliases             []APIProjectAlias      `json:"aliases"`
	Variables           APIProjectVars         `json:"variables"`
	Subscriptions       []APISubscription      `json:"subscriptions"`
	DeleteSubscriptions []APIString            `json:"delete_subscriptions,omitempty"`
}

// ToService returns a service layer ProjectRef using the data from APIProjectRef
func (p *APIProjectRef) ToService() (interface{}, error) {

	commitQueue, err := p.CommitQueue.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "can't convert commit queue params")
	}

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
		DeactivatePrevious:   p.DeactivatePrevious,
		TracksPushEvents:     p.TracksPushEvents,
		PRTestingEnabled:     p.PRTestingEnabled,
		CommitQueue:          commitQueue.(model.CommitQueueParams),
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
			GenerateFile:      FromAPIString(t.GenerateFile),
			Command:           FromAPIString(t.Command),
			Alias:             FromAPIString(t.Alias),
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
	p.DeactivatePrevious = projectRef.DeactivatePrevious
	p.TracksPushEvents = projectRef.TracksPushEvents
	p.PRTestingEnabled = projectRef.PRTestingEnabled
	p.CommitQueue = cq
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
			GenerateFile:      ToAPIString(t.GenerateFile),
			Command:           ToAPIString(t.Command),
			Alias:             ToAPIString(t.Alias),
			DateCutoff:        t.DateCutoff,
		})
	}
	p.Triggers = triggers

	return nil
}
