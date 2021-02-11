package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// publicProjectFields are the fields needed by the UI
// on base_angular and the menu
type UIProjectFields struct {
	Id          string `json:"id"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"display_name"`
	Repo        string `json:"repo_name"`
	Owner       string `json:"owner_name"`
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
	Enabled     *bool   `json:"enabled"`
	MergeMethod *string `json:"merge_method"`
	Message     *string `json:"message"`
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
	cqParams.MergeMethod = utility.ToStringPtr(params.MergeMethod)
	cqParams.Message = utility.ToStringPtr(params.Message)

	return nil
}

func (cqParams *APICommitQueueParams) ToService() (interface{}, error) {
	serviceParams := model.CommitQueueParams{}
	serviceParams.Enabled = cqParams.Enabled
	serviceParams.MergeMethod = utility.FromStringPtr(cqParams.MergeMethod)
	serviceParams.Message = utility.FromStringPtr(cqParams.Message)

	return serviceParams, nil
}

type APITaskSyncOptions struct {
	ConfigEnabled *bool `json:"config_enabled"`
	PatchEnabled  *bool `json:"patch_enabled"`
}

func (opts *APITaskSyncOptions) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.TaskSyncOptions:
		opts.ConfigEnabled = v.ConfigEnabled
		opts.PatchEnabled = v.PatchEnabled
		return nil
	default:
		return errors.Errorf("invalid type '%T' for API S3 task sync options", v)
	}
}

func (opts *APITaskSyncOptions) ToService() (interface{}, error) {
	return model.TaskSyncOptions{
		ConfigEnabled: opts.ConfigEnabled,
		PatchEnabled:  opts.PatchEnabled,
	}, nil
}

type APIWorkstationConfig struct {
	SetupCommands []APIWorkstationSetupCommand `bson:"setup_commands" json:"setup_commands"`
	GitClone      bool                         `bson:"git_clone" json:"git_clone"`
}

type APIWorkstationSetupCommand struct {
	Command   *string `bson:"command" json:"command"`
	Directory *string `bson:"directory" json:"directory"`
}

func (c *APIWorkstationConfig) ToService() (interface{}, error) {
	res := model.WorkstationConfig{}
	res.GitClone = c.GitClone
	for _, apiCmd := range c.SetupCommands {
		cmd := model.WorkstationSetupCommand{}
		cmd.Command = utility.FromStringPtr(apiCmd.Command)
		cmd.Directory = utility.FromStringPtr(apiCmd.Directory)
		res.SetupCommands = append(res.SetupCommands, cmd)
	}
	return res, nil
}

func (c *APIWorkstationConfig) BuildFromService(h interface{}) error {
	var config model.WorkstationConfig
	switch h.(type) {
	case model.WorkstationConfig:
		config = h.(model.WorkstationConfig)
	case *model.WorkstationConfig:
		config = *h.(*model.WorkstationConfig)
	}

	c.GitClone = config.GitClone
	for _, cmd := range config.SetupCommands {
		apiCmd := APIWorkstationSetupCommand{}
		apiCmd.Command = utility.ToStringPtr(cmd.Command)
		apiCmd.Directory = utility.ToStringPtr(cmd.Directory)
		c.SetupCommands = append(c.SetupCommands, apiCmd)
	}
	return nil
}

type APIParameterInfo struct {
	Key         *string `json:"key"`
	Value       *string `json:"value"`
	Description *string `json:"description"`
}

func (c *APIParameterInfo) ToService() (interface{}, error) {
	res := model.ParameterInfo{}
	res.Key = utility.FromStringPtr(c.Key)
	res.Value = utility.FromStringPtr(c.Value)
	res.Description = utility.FromStringPtr(c.Description)
	return res, nil
}

func (c *APIParameterInfo) BuildFromService(h interface{}) error {
	var info model.ParameterInfo
	switch h.(type) {
	case model.ParameterInfo:
		info = h.(model.ParameterInfo)
	case *model.ParameterInfo:
		info = *h.(*model.ParameterInfo)
	}

	c.Key = utility.ToStringPtr(info.Key)
	c.Value = utility.ToStringPtr(info.Value)
	c.Description = utility.ToStringPtr(info.Description)
	return nil
}

type APIProjectRef struct {
	Id                          *string              `json:"id"`
	Owner                       *string              `json:"owner_name"`
	Repo                        *string              `json:"repo_name"`
	Branch                      *string              `json:"branch_name"`
	Enabled                     *bool                `json:"enabled"`
	Private                     *bool                `json:"private"`
	BatchTime                   int                  `json:"batch_time"`
	RemotePath                  *string              `json:"remote_path"`
	SpawnHostScriptPath         *string              `json:"spawn_host_script_path"`
	Identifier                  *string              `json:"identifier"`
	DisplayName                 *string              `json:"display_name"`
	DeactivatePrevious          *bool                `json:"deactivate_previous"`
	TracksPushEvents            bool                 `json:"tracks_push_events"`
	PRTestingEnabled            *bool                `json:"pr_testing_enabled"`
	GitTagVersionsEnabled       *bool                `json:"git_tag_versions_enabled"`
	UseRepoSettings             bool                 `json:"use_repo_settings"`
	RepoRefId                   *string              `json:"repo_ref_id"`
	DefaultLogger               *string              `json:"default_logger"`
	CommitQueue                 APICommitQueueParams `json:"commit_queue"`
	TaskSync                    APITaskSyncOptions   `json:"task_sync"`
	Hidden                      *bool                `json:"hidden"`
	PatchingDisabled            *bool                `json:"patching_disabled"`
	RepotrackerDisabled         *bool                `json:"repotracker_disabled"`
	DispatchingDisabled         *bool                `json:"dispatching_disabled"`
	DisabledStatsCache          *bool                `json:"disabled_stats_cache"`
	FilesIgnoredFromCache       []*string            `json:"files_ignored_from_cache,omitempty"`
	Admins                      []*string            `json:"admins"`
	DeleteAdmins                []*string            `json:"delete_admins,omitempty"`
	GitTagAuthorizedUsers       []*string            `json:"git_tag_authorized_users" bson:"git_tag_authorized_users"`
	DeleteGitTagAuthorizedUsers []*string            `json:"delete_git_tag_authorized_users,omitempty" bson:"delete_git_tag_authorized_users,omitempty"`
	GitTagAuthorizedTeams       []*string            `json:"git_tag_authorized_teams" bson:"git_tag_authorized_teams"`
	DeleteGitTagAuthorizedTeams []*string            `json:"delete_git_tag_authorized_teams,omitempty" bson:"delete_git_tag_authorized_teams,omitempty"`
	NotifyOnBuildFailure        *bool                `json:"notify_on_failure"`

	Revision            *string                `json:"revision"`
	Triggers            []APITriggerDefinition `json:"triggers"`
	Aliases             []APIProjectAlias      `json:"aliases"`
	Variables           APIProjectVars         `json:"variables"`
	WorkstationConfig   APIWorkstationConfig   `json:"workstation_config"`
	Subscriptions       []APISubscription      `json:"subscriptions"`
	DeleteSubscriptions []*string              `json:"delete_subscriptions,omitempty"`
}

// ToService returns a service layer ProjectRef using the data from APIProjectRef
func (p *APIProjectRef) ToService() (interface{}, error) {

	commitQueue, err := p.CommitQueue.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "can't convert commit queue params")
	}

	i, err := p.TaskSync.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert API task sync options to service representation")
	}
	taskSync, ok := i.(model.TaskSyncOptions)
	if !ok {
		return nil, errors.Errorf("expected task sync options but was actually '%T'", i)
	}

	i, err = p.WorkstationConfig.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert API workstation config")
	}
	workstationConfig, ok := i.(model.WorkstationConfig)
	if !ok {
		return nil, errors.Errorf("expected workstation config but was actually '%T'", i)
	}

	projectRef := model.ProjectRef{
		Owner:                 utility.FromStringPtr(p.Owner),
		Repo:                  utility.FromStringPtr(p.Repo),
		Branch:                utility.FromStringPtr(p.Branch),
		Enabled:               p.Enabled,
		Private:               p.Private,
		BatchTime:             p.BatchTime,
		RemotePath:            utility.FromStringPtr(p.RemotePath),
		Id:                    utility.FromStringPtr(p.Id),
		Identifier:            utility.FromStringPtr(p.Identifier),
		DisplayName:           utility.FromStringPtr(p.DisplayName),
		DeactivatePrevious:    p.DeactivatePrevious,
		TracksPushEvents:      p.TracksPushEvents,
		DefaultLogger:         utility.FromStringPtr(p.DefaultLogger),
		PRTestingEnabled:      p.PRTestingEnabled,
		GitTagVersionsEnabled: p.GitTagVersionsEnabled,
		UseRepoSettings:       p.UseRepoSettings,
		RepoRefId:             utility.FromStringPtr(p.RepoRefId),
		CommitQueue:           commitQueue.(model.CommitQueueParams),
		TaskSync:              taskSync,
		WorkstationConfig:     workstationConfig,
		Hidden:                p.Hidden,
		PatchingDisabled:      p.PatchingDisabled,
		RepotrackerDisabled:   p.RepotrackerDisabled,
		DispatchingDisabled:   p.DispatchingDisabled,
		DisabledStatsCache:    p.DisabledStatsCache,
		FilesIgnoredFromCache: utility.FromStringPtrSlice(p.FilesIgnoredFromCache),
		NotifyOnBuildFailure:  p.NotifyOnBuildFailure,
		SpawnHostScriptPath:   utility.FromStringPtr(p.SpawnHostScriptPath),
		Admins:                utility.FromStringPtrSlice(p.Admins),
		GitTagAuthorizedUsers: utility.FromStringPtrSlice(p.GitTagAuthorizedUsers),
		GitTagAuthorizedTeams: utility.FromStringPtrSlice(p.GitTagAuthorizedTeams),
	}

	// Copy triggers
	triggers := []model.TriggerDefinition{}
	for _, t := range p.Triggers {
		triggers = append(triggers, model.TriggerDefinition{
			Project:           utility.FromStringPtr(t.Project),
			Level:             utility.FromStringPtr(t.Level),
			DefinitionID:      utility.FromStringPtr(t.DefinitionID),
			BuildVariantRegex: utility.FromStringPtr(t.BuildVariantRegex),
			TaskRegex:         utility.FromStringPtr(t.TaskRegex),
			Status:            utility.FromStringPtr(t.Status),
			ConfigFile:        utility.FromStringPtr(t.ConfigFile),
			GenerateFile:      utility.FromStringPtr(t.GenerateFile),
			Command:           utility.FromStringPtr(t.Command),
			Alias:             utility.FromStringPtr(t.Alias),
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

	var taskSync APITaskSyncOptions
	if err := taskSync.BuildFromService(projectRef.TaskSync); err != nil {
		return errors.Wrap(err, "cannot convert task sync options to API representation")
	}

	workstationConfig := APIWorkstationConfig{}
	if err := workstationConfig.BuildFromService(projectRef.WorkstationConfig); err != nil {
		return errors.Wrap(err, "cannot convert workstation config")
	}

	p.Owner = utility.ToStringPtr(projectRef.Owner)
	p.Repo = utility.ToStringPtr(projectRef.Repo)
	p.Branch = utility.ToStringPtr(projectRef.Branch)
	p.Enabled = projectRef.Enabled
	p.Private = projectRef.Private
	p.BatchTime = projectRef.BatchTime
	p.RemotePath = utility.ToStringPtr(projectRef.RemotePath)
	p.Id = utility.ToStringPtr(projectRef.Id)
	p.Identifier = utility.ToStringPtr(projectRef.Identifier)
	p.DisplayName = utility.ToStringPtr(projectRef.DisplayName)
	p.DeactivatePrevious = projectRef.DeactivatePrevious
	p.TracksPushEvents = projectRef.TracksPushEvents
	p.DefaultLogger = utility.ToStringPtr(projectRef.DefaultLogger)
	p.PRTestingEnabled = projectRef.PRTestingEnabled
	p.GitTagVersionsEnabled = projectRef.GitTagVersionsEnabled
	p.UseRepoSettings = projectRef.UseRepoSettings
	p.RepoRefId = utility.ToStringPtr(projectRef.RepoRefId)
	p.CommitQueue = cq
	p.TaskSync = taskSync
	p.WorkstationConfig = workstationConfig
	p.Hidden = projectRef.Hidden
	p.PatchingDisabled = projectRef.PatchingDisabled
	p.RepotrackerDisabled = projectRef.RepotrackerDisabled
	p.DispatchingDisabled = projectRef.DispatchingDisabled
	p.DisabledStatsCache = projectRef.DisabledStatsCache
	p.FilesIgnoredFromCache = utility.ToStringPtrSlice(projectRef.FilesIgnoredFromCache)
	p.NotifyOnBuildFailure = projectRef.NotifyOnBuildFailure
	p.SpawnHostScriptPath = utility.ToStringPtr(projectRef.SpawnHostScriptPath)
	p.Admins = utility.ToStringPtrSlice(projectRef.Admins)
	p.GitTagAuthorizedUsers = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedUsers)
	p.GitTagAuthorizedTeams = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedTeams)

	// Copy triggers
	triggers := []APITriggerDefinition{}
	for _, t := range projectRef.Triggers {
		triggers = append(triggers, APITriggerDefinition{
			Project:           utility.ToStringPtr(t.Project),
			Level:             utility.ToStringPtr(t.Level),
			DefinitionID:      utility.ToStringPtr(t.DefinitionID),
			BuildVariantRegex: utility.ToStringPtr(t.BuildVariantRegex),
			TaskRegex:         utility.ToStringPtr(t.TaskRegex),
			Status:            utility.ToStringPtr(t.Status),
			ConfigFile:        utility.ToStringPtr(t.ConfigFile),
			GenerateFile:      utility.ToStringPtr(t.GenerateFile),
			Command:           utility.ToStringPtr(t.Command),
			Alias:             utility.ToStringPtr(t.Alias),
			DateCutoff:        t.DateCutoff,
		})
	}
	p.Triggers = triggers

	return nil
}
