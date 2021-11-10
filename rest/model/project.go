package model

import (
	"fmt"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
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

func (t *APITriggerDefinition) ToService() (interface{}, error) {
	return model.TriggerDefinition{
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
	}, nil
}

func (t *APITriggerDefinition) BuildFromService(h interface{}) error {
	var triggerDef model.TriggerDefinition
	switch h.(type) {
	case model.TriggerDefinition:
		triggerDef = h.(model.TriggerDefinition)
	case *model.TriggerDefinition:
		triggerDef = *h.(*model.TriggerDefinition)
	default:
		return errors.Errorf("Invalid trigger definition of type '%T'", h)
	}
	t.Project = utility.ToStringPtr(triggerDef.Project)
	t.Level = utility.ToStringPtr(triggerDef.Level)
	t.DefinitionID = utility.ToStringPtr(triggerDef.DefinitionID)
	t.BuildVariantRegex = utility.ToStringPtr(triggerDef.BuildVariantRegex)
	t.TaskRegex = utility.ToStringPtr(triggerDef.TaskRegex)
	t.Status = utility.ToStringPtr(triggerDef.Status)
	t.ConfigFile = utility.ToStringPtr(triggerDef.ConfigFile)
	t.GenerateFile = utility.ToStringPtr(triggerDef.GenerateFile)
	t.Command = utility.ToStringPtr(triggerDef.Command)
	t.Alias = utility.ToStringPtr(triggerDef.Alias)
	t.DateCutoff = triggerDef.DateCutoff
	return nil
}

type APIPatchTriggerDefinition struct {
	Alias                  *string            `json:"alias"`
	ChildProject           *string            `json:"child_project"` // deprecated
	ChildProjectId         *string            `json:"child_project_id"`
	ChildProjectIdentifier *string            `json:"child_project_identifier"`
	TaskSpecifiers         []APITaskSpecifier `json:"task_specifiers"`
	Status                 *string            `json:"status,omitempty"`
	ParentAsModule         *string            `json:"parent_as_module,omitempty"`
	VariantsTasks          []VariantTask      `json:"variants_tasks,omitempty"`
}

func (t *APIPatchTriggerDefinition) BuildFromService(h interface{}) error {
	var def patch.PatchTriggerDefinition
	switch h.(type) {
	case patch.PatchTriggerDefinition:
		def = h.(patch.PatchTriggerDefinition)
	case *patch.PatchTriggerDefinition:
		def = *h.(*patch.PatchTriggerDefinition)
	default:
		return errors.Errorf("Invalid patch trigger definition of type '%T'", h)
	}
	t.ChildProject = utility.ToStringPtr(def.ChildProject)
	t.Alias = utility.ToStringPtr(def.Alias)
	t.Status = utility.ToStringPtr(def.Status)
	t.ParentAsModule = utility.ToStringPtr(def.ParentAsModule)
	var specifiers []APITaskSpecifier
	for _, ts := range def.TaskSpecifiers {
		specifier := APITaskSpecifier{}
		if err := specifier.BuildFromService(ts); err != nil {
			return errors.Wrap(err, "cannot convert task specifier")
		}
		specifiers = append(specifiers, specifier)
	}
	t.TaskSpecifiers = specifiers
	return nil
}

func (t *APIPatchTriggerDefinition) ToService() (interface{}, error) {
	trigger := patch.PatchTriggerDefinition{}

	trigger.ChildProject = utility.FromStringPtr(t.ChildProject)
	trigger.Status = utility.FromStringPtr(t.Status)
	trigger.Alias = utility.FromStringPtr(t.Alias)
	trigger.ParentAsModule = utility.FromStringPtr(t.ParentAsModule)
	var specifiers []patch.TaskSpecifier
	for _, ts := range t.TaskSpecifiers {
		i, err := ts.ToService()
		specifier, ok := i.(patch.TaskSpecifier)
		if err != nil {
			return nil, errors.Wrap(err, "cannot convert API task specifier")
		}
		if !ok {
			return nil, errors.Errorf("expected patch trigger definition but was actually '%T'", i)
		}
		specifiers = append(specifiers, specifier)
	}
	trigger.TaskSpecifiers = specifiers
	return trigger, nil
}

type APITaskSpecifier struct {
	PatchAlias   *string `json:"patch_alias,omitempty"`
	TaskRegex    *string `json:"task_regex,omitempty"`
	VariantRegex *string `json:"variant_regex,omitempty"`
}

func (ts *APITaskSpecifier) BuildFromService(h interface{}) error {
	var def patch.TaskSpecifier
	switch h.(type) {
	case patch.TaskSpecifier:
		def = h.(patch.TaskSpecifier)
	case *patch.TaskSpecifier:
		def = *h.(*patch.TaskSpecifier)
	default:
		return errors.Errorf("Invalid task specifier of type '%T'", h)
	}
	ts.PatchAlias = utility.ToStringPtr(def.PatchAlias)
	ts.TaskRegex = utility.ToStringPtr(def.TaskRegex)
	ts.VariantRegex = utility.ToStringPtr(def.VariantRegex)
	return nil
}

func (t *APITaskSpecifier) ToService() (interface{}, error) {
	specifier := patch.TaskSpecifier{}

	specifier.PatchAlias = utility.FromStringPtr(t.PatchAlias)
	specifier.TaskRegex = utility.FromStringPtr(t.TaskRegex)
	specifier.VariantRegex = utility.FromStringPtr(t.VariantRegex)
	return specifier, nil
}

type APIPeriodicBuildDefinition struct {
	ID            *string    `json:"id"`
	ConfigFile    *string    `json:"config_file"`
	IntervalHours *int       `son:"interval_hours"`
	Alias         *string    `son:"alias,omitempty"`
	Message       *string    `json:"message,omitempty"`
	NextRunTime   *time.Time `json:"next_run_time,omitempty"`
}

type APICommitQueueParams struct {
	Enabled       *bool   `json:"enabled"`
	RequireSigned *bool   `json:"require_signed"`
	MergeMethod   *string `json:"merge_method"`
	Message       *string `json:"message"`
}

func (bd *APIPeriodicBuildDefinition) ToService() (interface{}, error) {
	buildDef := model.PeriodicBuildDefinition{}
	buildDef.ID = utility.FromStringPtr(bd.ID)
	buildDef.ConfigFile = utility.FromStringPtr(bd.ConfigFile)
	buildDef.IntervalHours = utility.FromIntPtr(bd.IntervalHours)
	buildDef.Alias = utility.FromStringPtr(bd.Alias)
	buildDef.Message = utility.FromStringPtr(bd.Message)
	buildDef.NextRunTime = utility.FromTimePtr(bd.NextRunTime)
	return buildDef, nil
}

func (bd *APIPeriodicBuildDefinition) BuildFromService(h interface{}) error {
	var params model.PeriodicBuildDefinition
	switch h.(type) {
	case model.PeriodicBuildDefinition:
		params = h.(model.PeriodicBuildDefinition)
	case *model.PeriodicBuildDefinition:
		params = *h.(*model.PeriodicBuildDefinition)
	default:
		return errors.Errorf("Invalid commit queue params of type '%T'", h)
	}
	bd.ID = utility.ToStringPtr(params.ID)
	bd.ConfigFile = utility.ToStringPtr(params.ConfigFile)
	bd.IntervalHours = utility.ToIntPtr(params.IntervalHours)
	bd.Alias = utility.ToStringPtr(params.Alias)
	bd.Message = utility.ToStringPtr(params.Message)
	bd.NextRunTime = utility.ToTimePtr(params.NextRunTime)
	return nil
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

	cqParams.Enabled = utility.BoolPtrCopy(params.Enabled)
	cqParams.RequireSigned = utility.BoolPtrCopy(params.RequireSigned)
	cqParams.MergeMethod = utility.ToStringPtr(params.MergeMethod)
	cqParams.Message = utility.ToStringPtr(params.Message)

	return nil
}

func (cqParams *APICommitQueueParams) ToService() (interface{}, error) {
	serviceParams := model.CommitQueueParams{}
	serviceParams.Enabled = utility.BoolPtrCopy(cqParams.Enabled)
	serviceParams.RequireSigned = utility.BoolPtrCopy(cqParams.RequireSigned)
	serviceParams.MergeMethod = utility.FromStringPtr(cqParams.MergeMethod)
	serviceParams.Message = utility.FromStringPtr(cqParams.Message)

	return serviceParams, nil
}

type APIBuildBaronSettings struct {
	TicketCreateProject     *string   `bson:"ticket_create_project" json:"ticket_create_project"`
	TicketSearchProjects    []*string `bson:"ticket_search_projects" json:"ticket_search_projects"`
	BFSuggestionServer      *string   `bson:"bf_suggestion_server" json:"bf_suggestion_server"`
	BFSuggestionUsername    *string   `bson:"bf_suggestion_username" json:"bf_suggestion_username"`
	BFSuggestionPassword    *string   `bson:"bf_suggestion_password" json:"bf_suggestion_password"`
	BFSuggestionTimeoutSecs *int      `bson:"bf_suggestion_timeout_secs" json:"bf_suggestion_timeout_secs"`
	BFSuggestionFeaturesURL *string   `bson:"bf_suggestion_features_url" json:"bf_suggestion_features_url"`
}

func (bb *APIBuildBaronSettings) BuildFromService(h interface{}) error {
	var def evergreen.BuildBaronSettings
	switch h.(type) {
	case evergreen.BuildBaronSettings:
		def = h.(evergreen.BuildBaronSettings)
	case *evergreen.BuildBaronSettings:
		def = *h.(*evergreen.BuildBaronSettings)
	default:
		return errors.Errorf("Invalid build baron config of type '%T'", h)
	}
	bb.TicketCreateProject = utility.ToStringPtr(def.TicketCreateProject)
	bb.TicketSearchProjects = utility.ToStringPtrSlice(def.TicketSearchProjects)
	bb.BFSuggestionServer = utility.ToStringPtr(def.BFSuggestionServer)
	bb.BFSuggestionUsername = utility.ToStringPtr(def.BFSuggestionUsername)
	bb.BFSuggestionPassword = utility.ToStringPtr(def.BFSuggestionPassword)
	bb.BFSuggestionTimeoutSecs = utility.ToIntPtr(def.BFSuggestionTimeoutSecs)
	bb.BFSuggestionFeaturesURL = utility.ToStringPtr(def.BFSuggestionFeaturesURL)
	return nil
}

func (bb *APIBuildBaronSettings) ToService() (interface{}, error) {
	buildbaron := evergreen.BuildBaronSettings{}

	buildbaron.TicketCreateProject = utility.FromStringPtr(bb.TicketCreateProject)
	buildbaron.TicketSearchProjects = utility.FromStringPtrSlice(bb.TicketSearchProjects)
	buildbaron.BFSuggestionServer = utility.FromStringPtr(bb.BFSuggestionServer)
	buildbaron.BFSuggestionUsername = utility.FromStringPtr(bb.BFSuggestionUsername)
	buildbaron.BFSuggestionPassword = utility.FromStringPtr(bb.BFSuggestionPassword)
	buildbaron.BFSuggestionTimeoutSecs = utility.FromIntPtr(bb.BFSuggestionTimeoutSecs)
	buildbaron.BFSuggestionFeaturesURL = utility.FromStringPtr(bb.BFSuggestionFeaturesURL)
	return buildbaron, nil
}

type APITaskAnnotationSettings struct {
	JiraCustomFields  []APIJiraField `bson:"jira_custom_fields" json:"jira_custom_fields"`
	FileTicketWebHook APIWebHook     `bson:"web_hook" json:"web_hook"`
}

type APIWebHook struct {
	Endpoint *string `bson:"endpoint" json:"endpoint"`
	Secret   *string `bson:"secret" json:"secret"`
}

type APIJiraField struct {
	Field       *string `bson:"field" json:"field"`
	DisplayText *string `bson:"display_text" json:"display_text"`
}

func (ta *APITaskAnnotationSettings) ToService() (interface{}, error) {
	res := evergreen.AnnotationsSettings{}
	webhook := evergreen.WebHook{}
	webhook.Secret = utility.FromStringPtr(ta.FileTicketWebHook.Secret)
	webhook.Endpoint = utility.FromStringPtr(ta.FileTicketWebHook.Endpoint)
	res.FileTicketWebHook = webhook
	for _, apiJiraField := range ta.JiraCustomFields {
		jiraField := evergreen.JiraField{}
		jiraField.Field = utility.FromStringPtr(apiJiraField.Field)
		jiraField.DisplayText = utility.FromStringPtr(apiJiraField.DisplayText)
		res.JiraCustomFields = append(res.JiraCustomFields, jiraField)
	}
	return res, nil
}

func (ta *APITaskAnnotationSettings) BuildFromService(h interface{}) error {
	var config evergreen.AnnotationsSettings
	switch h.(type) {
	case evergreen.AnnotationsSettings:
		config = h.(evergreen.AnnotationsSettings)
	case *evergreen.AnnotationsSettings:
		config = *h.(*evergreen.AnnotationsSettings)
	}

	apiWebhook := APIWebHook{}
	apiWebhook.Secret = utility.ToStringPtr(config.FileTicketWebHook.Secret)
	apiWebhook.Endpoint = utility.ToStringPtr(config.FileTicketWebHook.Endpoint)
	ta.FileTicketWebHook = apiWebhook
	for _, jiraField := range config.JiraCustomFields {
		apiJiraField := APIJiraField{}
		apiJiraField.Field = utility.ToStringPtr(jiraField.Field)
		apiJiraField.DisplayText = utility.ToStringPtr(jiraField.DisplayText)
		ta.JiraCustomFields = append(ta.JiraCustomFields, apiJiraField)
	}
	return nil
}

type APITaskSyncOptions struct {
	ConfigEnabled *bool `json:"config_enabled"`
	PatchEnabled  *bool `json:"patch_enabled"`
}

func (opts *APITaskSyncOptions) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case model.TaskSyncOptions:
		opts.ConfigEnabled = utility.BoolPtrCopy(v.ConfigEnabled)
		opts.PatchEnabled = utility.BoolPtrCopy(v.PatchEnabled)
		return nil
	default:
		return errors.Errorf("invalid type '%T' for API S3 task sync options", v)
	}
}

func (opts *APITaskSyncOptions) ToService() (interface{}, error) {
	return model.TaskSyncOptions{
		ConfigEnabled: utility.BoolPtrCopy(opts.ConfigEnabled),
		PatchEnabled:  utility.BoolPtrCopy(opts.PatchEnabled),
	}, nil
}

type APIWorkstationConfig struct {
	SetupCommands []APIWorkstationSetupCommand `bson:"setup_commands" json:"setup_commands"`
	GitClone      *bool                        `bson:"git_clone" json:"git_clone"`
}

type APIWorkstationSetupCommand struct {
	Command   *string `bson:"command" json:"command"`
	Directory *string `bson:"directory" json:"directory"`
}

func (c *APIWorkstationConfig) ToService() (interface{}, error) {
	res := model.WorkstationConfig{}
	res.GitClone = utility.BoolPtrCopy(c.GitClone)
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

	c.GitClone = utility.BoolPtrCopy(config.GitClone)
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
	Id                          *string                   `json:"id"`
	Owner                       *string                   `json:"owner_name"`
	Repo                        *string                   `json:"repo_name"`
	Branch                      *string                   `json:"branch_name"`
	Enabled                     *bool                     `json:"enabled"`
	Private                     *bool                     `json:"private"`
	BatchTime                   int                       `json:"batch_time"`
	RemotePath                  *string                   `json:"remote_path"`
	SpawnHostScriptPath         *string                   `json:"spawn_host_script_path"`
	Identifier                  *string                   `json:"identifier"`
	DisplayName                 *string                   `json:"display_name"`
	DeactivatePrevious          *bool                     `json:"deactivate_previous"`
	TracksPushEvents            *bool                     `json:"tracks_push_events"`
	PRTestingEnabled            *bool                     `json:"pr_testing_enabled"`
	GitTagVersionsEnabled       *bool                     `json:"git_tag_versions_enabled"`
	GithubChecksEnabled         *bool                     `json:"github_checks_enabled"`
	CedarTestResultsEnabled     *bool                     `json:"cedar_test_results_enabled"`
	UseRepoSettings             bool                      `json:"use_repo_settings"`
	RepoRefId                   *string                   `json:"repo_ref_id"`
	DefaultLogger               *string                   `json:"default_logger"`
	CommitQueue                 APICommitQueueParams      `json:"commit_queue"`
	TaskSync                    APITaskSyncOptions        `json:"task_sync"`
	TaskAnnotationSettings      APITaskAnnotationSettings `json:"task_annotation_settings"`
	BuildBaronSettings          APIBuildBaronSettings     `json:"build_baron_settings"`
	PerfEnabled                 *bool                     `json:"perf_enabled"`
	Hidden                      *bool                     `json:"hidden"`
	PatchingDisabled            *bool                     `json:"patching_disabled"`
	RepotrackerDisabled         *bool                     `json:"repotracker_disabled"`
	DispatchingDisabled         *bool                     `json:"dispatching_disabled"`
	DisabledStatsCache          *bool                     `json:"disabled_stats_cache"`
	FilesIgnoredFromCache       []*string                 `json:"files_ignored_from_cache"`
	Admins                      []*string                 `json:"admins"`
	DeleteAdmins                []*string                 `json:"delete_admins,omitempty"`
	GitTagAuthorizedUsers       []*string                 `json:"git_tag_authorized_users" bson:"git_tag_authorized_users"`
	DeleteGitTagAuthorizedUsers []*string                 `json:"delete_git_tag_authorized_users,omitempty" bson:"delete_git_tag_authorized_users,omitempty"`
	GitTagAuthorizedTeams       []*string                 `json:"git_tag_authorized_teams" bson:"git_tag_authorized_teams"`
	DeleteGitTagAuthorizedTeams []*string                 `json:"delete_git_tag_authorized_teams,omitempty" bson:"delete_git_tag_authorized_teams,omitempty"`
	NotifyOnBuildFailure        *bool                     `json:"notify_on_failure"`
	Restricted                  *bool                     `json:"restricted"`
	Revision                    *string                   `json:"revision"`

	Triggers             []APITriggerDefinition       `json:"triggers"`
	GithubTriggerAliases []*string                    `json:"github_trigger_aliases"`
	PatchTriggerAliases  []APIPatchTriggerDefinition  `json:"patch_trigger_aliases"`
	Aliases              []APIProjectAlias            `json:"aliases"`
	Variables            APIProjectVars               `json:"variables"`
	WorkstationConfig    APIWorkstationConfig         `json:"workstation_config"`
	Subscriptions        []APISubscription            `json:"subscriptions"`
	DeleteSubscriptions  []*string                    `json:"delete_subscriptions,omitempty"`
	PeriodicBuilds       []APIPeriodicBuildDefinition `json:"periodic_builds,omitempty"`
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

	i, err = p.BuildBaronSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert API buildbaron config")
	}
	buildBaronConfig, ok := i.(evergreen.BuildBaronSettings)
	if !ok {
		return nil, errors.Errorf("expected buildbaron config but was actually '%T'", i)
	}

	i, err = p.TaskAnnotationSettings.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "cannot convert API task annotations config")
	}
	taskAnnotationConfig, ok := i.(evergreen.AnnotationsSettings)
	if !ok {
		return nil, errors.Errorf("expected task annotations config but was actually '%T'", i)
	}

	projectRef := model.ProjectRef{
		Owner:                  utility.FromStringPtr(p.Owner),
		Repo:                   utility.FromStringPtr(p.Repo),
		Branch:                 utility.FromStringPtr(p.Branch),
		Enabled:                utility.BoolPtrCopy(p.Enabled),
		Private:                utility.BoolPtrCopy(p.Private),
		Restricted:             utility.BoolPtrCopy(p.Restricted),
		BatchTime:              p.BatchTime,
		RemotePath:             utility.FromStringPtr(p.RemotePath),
		Id:                     utility.FromStringPtr(p.Id),
		Identifier:             utility.FromStringPtr(p.Identifier),
		DisplayName:            utility.FromStringPtr(p.DisplayName),
		DeactivatePrevious:     utility.BoolPtrCopy(p.DeactivatePrevious),
		TracksPushEvents:       utility.BoolPtrCopy(p.TracksPushEvents),
		DefaultLogger:          utility.FromStringPtr(p.DefaultLogger),
		PRTestingEnabled:       utility.BoolPtrCopy(p.PRTestingEnabled),
		GitTagVersionsEnabled:  utility.BoolPtrCopy(p.GitTagVersionsEnabled),
		GithubChecksEnabled:    utility.BoolPtrCopy(p.GithubChecksEnabled),
		UseRepoSettings:        p.UseRepoSettings,
		RepoRefId:              utility.FromStringPtr(p.RepoRefId),
		CommitQueue:            commitQueue.(model.CommitQueueParams),
		TaskSync:               taskSync,
		WorkstationConfig:      workstationConfig,
		BuildBaronSettings:     buildBaronConfig,
		TaskAnnotationSettings: taskAnnotationConfig,
		PerfEnabled:            utility.BoolPtrCopy(p.PerfEnabled),
		Hidden:                 utility.BoolPtrCopy(p.Hidden),
		PatchingDisabled:       utility.BoolPtrCopy(p.PatchingDisabled),
		RepotrackerDisabled:    utility.BoolPtrCopy(p.RepotrackerDisabled),
		DispatchingDisabled:    utility.BoolPtrCopy(p.DispatchingDisabled),
		DisabledStatsCache:     utility.BoolPtrCopy(p.DisabledStatsCache),
		FilesIgnoredFromCache:  utility.FromStringPtrSlice(p.FilesIgnoredFromCache),
		NotifyOnBuildFailure:   utility.BoolPtrCopy(p.NotifyOnBuildFailure),
		SpawnHostScriptPath:    utility.FromStringPtr(p.SpawnHostScriptPath),
		Admins:                 utility.FromStringPtrSlice(p.Admins),
		GitTagAuthorizedUsers:  utility.FromStringPtrSlice(p.GitTagAuthorizedUsers),
		GitTagAuthorizedTeams:  utility.FromStringPtrSlice(p.GitTagAuthorizedTeams),
		GithubTriggerAliases:   utility.FromStringPtrSlice(p.GithubTriggerAliases),
	}

	// Copy triggers
	if p.Triggers != nil {
		triggers := []model.TriggerDefinition{}
		for _, t := range p.Triggers {
			i, err = t.ToService()
			if err != nil {
				return nil, errors.Wrap(err, "cannot convert API trigger definition")
			}
			newTrigger, ok := i.(model.TriggerDefinition)
			if !ok {
				return nil, errors.Errorf("expected trigger definition but was actually '%T'", i)
			}
			triggers = append(triggers, newTrigger)
		}
		projectRef.Triggers = triggers
	}

	// Copy periodic builds
	if p.PeriodicBuilds != nil {
		builds := []model.PeriodicBuildDefinition{}
		for _, t := range p.PeriodicBuilds {
			i, err = t.ToService()
			if err != nil {
				return nil, errors.Wrap(err, "cannot convert API periodic build")
			}
			newBuild, ok := i.(model.PeriodicBuildDefinition)
			if !ok {
				return nil, errors.Errorf("expected periodic build definition but was actually '%T'", i)
			}
			builds = append(builds, newBuild)
		}
		projectRef.PeriodicBuilds = builds
	}

	if p.PatchTriggerAliases != nil {
		patchTriggers := []patch.PatchTriggerDefinition{}
		for _, t := range p.PatchTriggerAliases {
			i, err = t.ToService()
			if err != nil {
				return nil, errors.Wrap(err, "cannot convert API patch trigger definition")
			}
			trigger, ok := i.(patch.PatchTriggerDefinition)
			if !ok {
				return nil, errors.Errorf("expected patch trigger definition but was actually '%T'", i)
			}
			patchTriggers = append(patchTriggers, trigger)
		}
		projectRef.PatchTriggerAliases = patchTriggers
	}
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

	p.Owner = utility.ToStringPtr(projectRef.Owner)
	p.Repo = utility.ToStringPtr(projectRef.Repo)
	p.Branch = utility.ToStringPtr(projectRef.Branch)
	p.Enabled = utility.BoolPtrCopy(projectRef.Enabled)
	p.Private = utility.BoolPtrCopy(projectRef.Private)
	p.Restricted = utility.BoolPtrCopy(projectRef.Restricted)
	p.BatchTime = projectRef.BatchTime
	p.RemotePath = utility.ToStringPtr(projectRef.RemotePath)
	p.Id = utility.ToStringPtr(projectRef.Id)
	p.Identifier = utility.ToStringPtr(projectRef.Identifier)
	p.DisplayName = utility.ToStringPtr(projectRef.DisplayName)
	p.DeactivatePrevious = projectRef.DeactivatePrevious
	p.TracksPushEvents = utility.BoolPtrCopy(projectRef.TracksPushEvents)
	p.DefaultLogger = utility.ToStringPtr(projectRef.DefaultLogger)
	p.PRTestingEnabled = utility.BoolPtrCopy(projectRef.PRTestingEnabled)
	p.GitTagVersionsEnabled = utility.BoolPtrCopy(projectRef.GitTagVersionsEnabled)
	p.GithubChecksEnabled = utility.BoolPtrCopy(projectRef.GithubChecksEnabled)
	p.UseRepoSettings = projectRef.UseRepoSettings
	p.RepoRefId = utility.ToStringPtr(projectRef.RepoRefId)
	p.PerfEnabled = utility.BoolPtrCopy(projectRef.PerfEnabled)
	p.Hidden = utility.BoolPtrCopy(projectRef.Hidden)
	p.PatchingDisabled = utility.BoolPtrCopy(projectRef.PatchingDisabled)
	p.RepotrackerDisabled = utility.BoolPtrCopy(projectRef.RepotrackerDisabled)
	p.DispatchingDisabled = utility.BoolPtrCopy(projectRef.DispatchingDisabled)
	p.DisabledStatsCache = utility.BoolPtrCopy(projectRef.DisabledStatsCache)
	p.FilesIgnoredFromCache = utility.ToStringPtrSlice(projectRef.FilesIgnoredFromCache)
	p.NotifyOnBuildFailure = utility.BoolPtrCopy(projectRef.NotifyOnBuildFailure)
	p.SpawnHostScriptPath = utility.ToStringPtr(projectRef.SpawnHostScriptPath)
	p.Admins = utility.ToStringPtrSlice(projectRef.Admins)
	p.GitTagAuthorizedUsers = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedUsers)
	p.GitTagAuthorizedTeams = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedTeams)
	p.GithubTriggerAliases = utility.ToStringPtrSlice(projectRef.GithubTriggerAliases)

	cq := APICommitQueueParams{}
	if err := cq.BuildFromService(projectRef.CommitQueue); err != nil {
		return errors.Wrap(err, "can't convert commit queue parameters")
	}
	p.CommitQueue = cq

	var taskSync APITaskSyncOptions
	if err := taskSync.BuildFromService(projectRef.TaskSync); err != nil {
		return errors.Wrap(err, "cannot convert task sync options to API representation")
	}
	p.TaskSync = taskSync

	workstationConfig := APIWorkstationConfig{}
	if err := workstationConfig.BuildFromService(projectRef.WorkstationConfig); err != nil {
		return errors.Wrap(err, "cannot convert workstation config")
	}
	p.WorkstationConfig = workstationConfig

	buildbaronConfig := APIBuildBaronSettings{}
	if err := buildbaronConfig.BuildFromService(projectRef.BuildBaronSettings); err != nil {
		return errors.Wrap(err, "cannot convert build baron config")
	}
	p.BuildBaronSettings = buildbaronConfig

	taskannotationConfig := APITaskAnnotationSettings{}
	if err := taskannotationConfig.BuildFromService(projectRef.TaskAnnotationSettings); err != nil {
		return errors.Wrap(err, "cannot convert task annotations config")
	}
	p.TaskAnnotationSettings = taskannotationConfig

	// Copy triggers
	if projectRef.Triggers != nil {
		triggers := []APITriggerDefinition{}
		for _, t := range projectRef.Triggers {
			apiTrigger := APITriggerDefinition{}
			if err := apiTrigger.BuildFromService(t); err != nil {
				return err
			}
			triggers = append(triggers, apiTrigger)
		}
		p.Triggers = triggers
	}

	// copy periodic builds
	if projectRef.PeriodicBuilds != nil {
		periodicBuilds := []APIPeriodicBuildDefinition{}
		for _, pb := range projectRef.PeriodicBuilds {
			periodicBuild := APIPeriodicBuildDefinition{}
			if err := periodicBuild.BuildFromService(pb); err != nil {
				return err
			}
			periodicBuilds = append(periodicBuilds)
		}
		p.PeriodicBuilds = periodicBuilds
	}

	if projectRef.PatchTriggerAliases != nil {
		patchTriggers := []APIPatchTriggerDefinition{}
		for _, t := range projectRef.PatchTriggerAliases {
			trigger := APIPatchTriggerDefinition{}
			if err := trigger.BuildFromService(t); err != nil {
				return errors.Wrap(err, "cannot convert trigger definition")
			}
			patchTriggers = append(patchTriggers, trigger)
		}
		p.PatchTriggerAliases = patchTriggers
	}

	return nil
}

// DefaultUnsetBooleans is used to set booleans to their default value.
func (pRef *APIProjectRef) DefaultUnsetBooleans() {
	reflected := reflect.ValueOf(pRef).Elem()
	recursivelyDefaultBooleans(reflected)
}

func recursivelyDefaultBooleans(structToSet reflect.Value) {
	var err error
	var i int
	defer func() {
		grip.Error(recovery.HandlePanicWithError(recover(), err, fmt.Sprintf("panicked for field '%d'", i)))
	}()
	falseType := reflect.TypeOf(false)
	// Iterate through each field of the struct.
	for i = 0; i < structToSet.NumField(); i++ {
		field := structToSet.Field(i)

		// If it's a boolean pointer, set the default recursively.
		if field.Type() == reflect.PtrTo(falseType) && util.IsFieldUndefined(field) {
			field.Set(reflect.New(falseType))

		} else if field.Kind() == reflect.Struct {
			recursivelyDefaultBooleans(field)
		}
	}
}
