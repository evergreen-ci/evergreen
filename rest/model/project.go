package model

import (
	"encoding/json"
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

type GetProjectTaskExecutionReq struct {
	TaskName     string   `json:"task_name"`
	BuildVariant string   `json:"build_variant"`
	Requesters   []string `json:"requesters"`

	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type ProjectTaskExecutionResp struct {
	NumCompleted int `json:"num_completed"`
}

type APITriggerDefinition struct {
	// project ID
	Project *string `json:"project"`
	// build or task
	Level *string `json:"level"`
	// unique ID
	DefinitionID *string `json:"definition_id"`
	// matching variants will trigger a build
	BuildVariantRegex *string `json:"variant_regex"`
	// matching tasks will trigger a build
	TaskRegex *string `json:"task_regex"`
	// status to trigger for (or "*")
	Status     *string `json:"status"`
	DateCutoff *int    `json:"date_cutoff"`
	// definition file
	ConfigFile                   *string `json:"config_file"`
	Alias                        *string `json:"alias"`
	UnscheduleDownstreamVersions *bool   `json:"unschedule_downstream_versions"`
}

func (t *APITriggerDefinition) ToService() model.TriggerDefinition {
	return model.TriggerDefinition{
		Project:                      utility.FromStringPtr(t.Project),
		Level:                        utility.FromStringPtr(t.Level),
		DefinitionID:                 utility.FromStringPtr(t.DefinitionID),
		BuildVariantRegex:            utility.FromStringPtr(t.BuildVariantRegex),
		TaskRegex:                    utility.FromStringPtr(t.TaskRegex),
		Status:                       utility.FromStringPtr(t.Status),
		ConfigFile:                   utility.FromStringPtr(t.ConfigFile),
		Alias:                        utility.FromStringPtr(t.Alias),
		UnscheduleDownstreamVersions: utility.FromBoolPtr(t.UnscheduleDownstreamVersions),
		DateCutoff:                   t.DateCutoff,
	}
}

func (t *APITriggerDefinition) BuildFromService(triggerDef model.TriggerDefinition) {
	t.Project = utility.ToStringPtr(triggerDef.Project)
	t.Level = utility.ToStringPtr(triggerDef.Level)
	t.DefinitionID = utility.ToStringPtr(triggerDef.DefinitionID)
	t.BuildVariantRegex = utility.ToStringPtr(triggerDef.BuildVariantRegex)
	t.TaskRegex = utility.ToStringPtr(triggerDef.TaskRegex)
	t.Status = utility.ToStringPtr(triggerDef.Status)
	t.ConfigFile = utility.ToStringPtr(triggerDef.ConfigFile)
	t.Alias = utility.ToStringPtr(triggerDef.Alias)
	t.UnscheduleDownstreamVersions = utility.ToBoolPtr(triggerDef.UnscheduleDownstreamVersions)
	t.DateCutoff = triggerDef.DateCutoff
}

type APIPatchTriggerDefinition struct {
	Alias                  *string            `json:"alias"`
	ChildProjectId         *string            `json:"child_project_id"`
	ChildProjectIdentifier *string            `json:"child_project_identifier"`
	TaskSpecifiers         []APITaskSpecifier `json:"task_specifiers"`
	Status                 *string            `json:"status,omitempty"`
	ParentAsModule         *string            `json:"parent_as_module,omitempty"`
	VariantsTasks          []VariantTask      `json:"variants_tasks,omitempty"`
}

func (t *APIPatchTriggerDefinition) BuildFromService(def patch.PatchTriggerDefinition) error {
	t.ChildProjectId = utility.ToStringPtr(def.ChildProject) // we store the real ID in the child project field
	identifier, err := model.GetIdentifierForProject(def.ChildProject)
	if err != nil {
		return errors.Wrapf(err, "getting identifier for child project '%s'", def.ChildProject)
	}
	t.ChildProjectIdentifier = utility.ToStringPtr(identifier)
	// not sure in which direction this should go
	t.Alias = utility.ToStringPtr(def.Alias)
	t.Status = utility.ToStringPtr(def.Status)
	t.ParentAsModule = utility.ToStringPtr(def.ParentAsModule)
	var specifiers []APITaskSpecifier
	for _, ts := range def.TaskSpecifiers {
		specifier := APITaskSpecifier{}
		specifier.BuildFromService(ts)
		specifiers = append(specifiers, specifier)
	}
	t.TaskSpecifiers = specifiers
	return nil
}

func (t *APIPatchTriggerDefinition) ToService() patch.PatchTriggerDefinition {
	trigger := patch.PatchTriggerDefinition{}

	trigger.ChildProject = utility.FromStringPtr(t.ChildProjectIdentifier) // we'll fix this to be the ID in case it's changed
	trigger.Status = utility.FromStringPtr(t.Status)
	trigger.Alias = utility.FromStringPtr(t.Alias)
	trigger.ParentAsModule = utility.FromStringPtr(t.ParentAsModule)
	var specifiers []patch.TaskSpecifier
	for _, ts := range t.TaskSpecifiers {
		specifiers = append(specifiers, ts.ToService())
	}
	trigger.TaskSpecifiers = specifiers
	return trigger
}

type APITaskSpecifier struct {
	PatchAlias   *string `json:"patch_alias,omitempty"`
	TaskRegex    *string `json:"task_regex,omitempty"`
	VariantRegex *string `json:"variant_regex,omitempty"`
}

func (ts *APITaskSpecifier) BuildFromService(def patch.TaskSpecifier) {
	ts.PatchAlias = utility.ToStringPtr(def.PatchAlias)
	ts.TaskRegex = utility.ToStringPtr(def.TaskRegex)
	ts.VariantRegex = utility.ToStringPtr(def.VariantRegex)
}

func (t *APITaskSpecifier) ToService() patch.TaskSpecifier {
	specifier := patch.TaskSpecifier{}

	specifier.PatchAlias = utility.FromStringPtr(t.PatchAlias)
	specifier.TaskRegex = utility.FromStringPtr(t.TaskRegex)
	specifier.VariantRegex = utility.FromStringPtr(t.VariantRegex)
	return specifier
}

type APIPeriodicBuildDefinition struct {
	ID            *string    `json:"id"`
	ConfigFile    *string    `json:"config_file"`
	IntervalHours *int       `json:"interval_hours"`
	Cron          *string    `json:"cron"`
	Alias         *string    `json:"alias,omitempty"`
	Message       *string    `json:"message,omitempty"`
	NextRunTime   *time.Time `json:"next_run_time,omitempty"`
}

type APIParsleyFilter struct {
	Expression    *string `json:"expression"`
	CaseSensitive *bool   `json:"case_sensitive"`
	ExactMatch    *bool   `json:"exact_match"`
}

func (t *APIParsleyFilter) ToService() model.ParsleyFilter {
	return model.ParsleyFilter{
		Expression:    utility.FromStringPtr(t.Expression),
		CaseSensitive: utility.FromBoolPtr(t.CaseSensitive),
		ExactMatch:    utility.FromBoolPtr(t.ExactMatch),
	}
}

func (t *APIParsleyFilter) BuildFromService(h model.ParsleyFilter) {
	t.Expression = utility.ToStringPtr(h.Expression)
	t.CaseSensitive = utility.ToBoolPtr(h.CaseSensitive)
	t.ExactMatch = utility.ToBoolPtr(h.ExactMatch)
}

type APIExternalLink struct {
	DisplayName *string   `json:"display_name"`
	Requesters  []*string `json:"requesters"`
	URLTemplate *string   `json:"url_template"`
}

func (t *APIExternalLink) ToService() model.ExternalLink {
	return model.ExternalLink{
		DisplayName: utility.FromStringPtr(t.DisplayName),
		Requesters:  utility.FromStringPtrSlice(t.Requesters),
		URLTemplate: utility.FromStringPtr(t.URLTemplate),
	}
}

func (t *APIExternalLink) BuildFromService(h model.ExternalLink) {
	t.DisplayName = utility.ToStringPtr(h.DisplayName)
	t.Requesters = utility.ToStringPtrSlice(h.Requesters)
	t.URLTemplate = utility.ToStringPtr(h.URLTemplate)
}

type APIProjectBanner struct {
	Theme evergreen.BannerTheme `json:"theme"`
	Text  *string               `json:"text"`
}

func (t *APIProjectBanner) ToService() model.ProjectBanner {
	return model.ProjectBanner{
		Theme: t.Theme,
		Text:  utility.FromStringPtr(t.Text),
	}
}

func (t *APIProjectBanner) BuildFromService(h model.ProjectBanner) {
	t.Theme = h.Theme
	t.Text = utility.ToStringPtr(h.Text)
}

func (bd *APIPeriodicBuildDefinition) ToService() model.PeriodicBuildDefinition {
	buildDef := model.PeriodicBuildDefinition{}
	buildDef.ID = utility.FromStringPtr(bd.ID)
	buildDef.ConfigFile = utility.FromStringPtr(bd.ConfigFile)
	buildDef.IntervalHours = utility.FromIntPtr(bd.IntervalHours)
	buildDef.Cron = utility.FromStringPtr(bd.Cron)
	buildDef.Alias = utility.FromStringPtr(bd.Alias)
	buildDef.Message = utility.FromStringPtr(bd.Message)
	buildDef.NextRunTime = utility.FromTimePtr(bd.NextRunTime)
	return buildDef
}

func (bd *APIPeriodicBuildDefinition) BuildFromService(params model.PeriodicBuildDefinition) {
	bd.ID = utility.ToStringPtr(params.ID)
	bd.ConfigFile = utility.ToStringPtr(params.ConfigFile)
	bd.IntervalHours = utility.ToIntPtr(params.IntervalHours)
	bd.Cron = utility.ToStringPtr(params.Cron)
	bd.Alias = utility.ToStringPtr(params.Alias)
	bd.Message = utility.ToStringPtr(params.Message)
	bd.NextRunTime = utility.ToTimePtr(params.NextRunTime)
}

type APICommitQueueParams struct {
	// Enable/disable the commit queue
	Enabled *bool `json:"enabled"`
	// method of merging (squash, merge, rebase
	MergeMethod *string `json:"merge_method"`
	// merge queue to use (EVERGREEN or GITHUB)
	MergeQueue model.MergeQueue `json:"merge_queue"`
	Message    *string          `json:"message"`
}

func (cqParams *APICommitQueueParams) BuildFromService(params model.CommitQueueParams) {
	cqParams.Enabled = utility.BoolPtrCopy(params.Enabled)
	cqParams.MergeMethod = utility.ToStringPtr(params.MergeMethod)
	cqParams.Message = utility.ToStringPtr(params.Message)

	if params.MergeQueue == "" {
		params.MergeQueue = model.MergeQueueEvergreen
	}
	cqParams.MergeQueue = params.MergeQueue
}

func (cqParams *APICommitQueueParams) ToService() model.CommitQueueParams {
	serviceParams := model.CommitQueueParams{}
	serviceParams.Enabled = utility.BoolPtrCopy(cqParams.Enabled)
	serviceParams.MergeMethod = utility.FromStringPtr(cqParams.MergeMethod)
	serviceParams.Message = utility.FromStringPtr(cqParams.Message)

	if cqParams.MergeQueue == "" {
		cqParams.MergeQueue = model.MergeQueueEvergreen
	}
	serviceParams.MergeQueue = cqParams.MergeQueue

	return serviceParams
}

type APIBuildBaronSettings struct {
	TicketCreateProject     *string   `bson:"ticket_create_project" json:"ticket_create_project"`
	TicketCreateIssueType   *string   `bson:"ticket_create_issue_type" json:"ticket_create_issue_type"`
	TicketSearchProjects    []*string `bson:"ticket_search_projects" json:"ticket_search_projects"`
	BFSuggestionServer      *string   `bson:"bf_suggestion_server" json:"bf_suggestion_server"`
	BFSuggestionUsername    *string   `bson:"bf_suggestion_username" json:"bf_suggestion_username"`
	BFSuggestionPassword    *string   `bson:"bf_suggestion_password" json:"bf_suggestion_password"`
	BFSuggestionTimeoutSecs *int      `bson:"bf_suggestion_timeout_secs" json:"bf_suggestion_timeout_secs"`
	BFSuggestionFeaturesURL *string   `bson:"bf_suggestion_features_url" json:"bf_suggestion_features_url"`
}

func (bb *APIBuildBaronSettings) BuildFromService(def evergreen.BuildBaronSettings) {
	bb.TicketCreateProject = utility.ToStringPtr(def.TicketCreateProject)
	bb.TicketCreateIssueType = utility.ToStringPtr(def.TicketCreateIssueType)
	bb.TicketSearchProjects = utility.ToStringPtrSlice(def.TicketSearchProjects)
	bb.BFSuggestionServer = utility.ToStringPtr(def.BFSuggestionServer)
	bb.BFSuggestionUsername = utility.ToStringPtr(def.BFSuggestionUsername)
	bb.BFSuggestionPassword = utility.ToStringPtr(def.BFSuggestionPassword)
	bb.BFSuggestionTimeoutSecs = utility.ToIntPtr(def.BFSuggestionTimeoutSecs)
	bb.BFSuggestionFeaturesURL = utility.ToStringPtr(def.BFSuggestionFeaturesURL)
}

func (bb *APIBuildBaronSettings) ToService() evergreen.BuildBaronSettings {
	buildBaron := evergreen.BuildBaronSettings{}
	buildBaron.TicketCreateProject = utility.FromStringPtr(bb.TicketCreateProject)
	buildBaron.TicketCreateIssueType = utility.FromStringPtr(bb.TicketCreateIssueType)
	buildBaron.TicketSearchProjects = utility.FromStringPtrSlice(bb.TicketSearchProjects)
	buildBaron.BFSuggestionServer = utility.FromStringPtr(bb.BFSuggestionServer)
	buildBaron.BFSuggestionUsername = utility.FromStringPtr(bb.BFSuggestionUsername)
	buildBaron.BFSuggestionPassword = utility.FromStringPtr(bb.BFSuggestionPassword)
	buildBaron.BFSuggestionTimeoutSecs = utility.FromIntPtr(bb.BFSuggestionTimeoutSecs)
	buildBaron.BFSuggestionFeaturesURL = utility.FromStringPtr(bb.BFSuggestionFeaturesURL)
	return buildBaron
}

type APITaskAnnotationSettings struct {
	JiraCustomFields  []APIJiraField `bson:"jira_custom_fields" json:"jira_custom_fields"`
	FileTicketWebhook APIWebHook     `bson:"web_hook" json:"web_hook"`
}

type APIWebHook struct {
	Endpoint *string `bson:"endpoint" json:"endpoint"`
	Secret   *string `bson:"secret" json:"secret"`
}

type APIJiraField struct {
	Field       *string `bson:"field" json:"field"`
	DisplayText *string `bson:"display_text" json:"display_text"`
}

func (ta *APITaskAnnotationSettings) ToService() evergreen.AnnotationsSettings {
	res := evergreen.AnnotationsSettings{}
	webhook := evergreen.WebHook{}
	webhook.Secret = utility.FromStringPtr(ta.FileTicketWebhook.Secret)
	webhook.Endpoint = utility.FromStringPtr(ta.FileTicketWebhook.Endpoint)
	res.FileTicketWebhook = webhook
	for _, apiJiraField := range ta.JiraCustomFields {
		jiraField := evergreen.JiraField{}
		jiraField.Field = utility.FromStringPtr(apiJiraField.Field)
		jiraField.DisplayText = utility.FromStringPtr(apiJiraField.DisplayText)
		res.JiraCustomFields = append(res.JiraCustomFields, jiraField)
	}
	return res
}

func (ta *APITaskAnnotationSettings) BuildFromService(config evergreen.AnnotationsSettings) {
	apiWebhook := APIWebHook{}
	apiWebhook.Secret = utility.ToStringPtr(config.FileTicketWebhook.Secret)
	apiWebhook.Endpoint = utility.ToStringPtr(config.FileTicketWebhook.Endpoint)
	ta.FileTicketWebhook = apiWebhook
	for _, jiraField := range config.JiraCustomFields {
		apiJiraField := APIJiraField{}
		apiJiraField.Field = utility.ToStringPtr(jiraField.Field)
		apiJiraField.DisplayText = utility.ToStringPtr(jiraField.DisplayText)
		ta.JiraCustomFields = append(ta.JiraCustomFields, apiJiraField)
	}
}

type APITaskSyncOptions struct {
	ConfigEnabled *bool `json:"config_enabled"`
	PatchEnabled  *bool `json:"patch_enabled"`
}

func (opts *APITaskSyncOptions) BuildFromService(in model.TaskSyncOptions) {
	opts.ConfigEnabled = utility.BoolPtrCopy(in.ConfigEnabled)
	opts.PatchEnabled = utility.BoolPtrCopy(in.PatchEnabled)
}

func (opts *APITaskSyncOptions) ToService() model.TaskSyncOptions {
	return model.TaskSyncOptions{
		ConfigEnabled: utility.BoolPtrCopy(opts.ConfigEnabled),
		PatchEnabled:  utility.BoolPtrCopy(opts.PatchEnabled),
	}
}

type APIWorkstationConfig struct {
	SetupCommands []APIWorkstationSetupCommand `bson:"setup_commands" json:"setup_commands"`
	GitClone      *bool                        `bson:"git_clone" json:"git_clone"`
}

type APIContainerSecret struct {
	Name         *string `json:"name"`
	ExternalName *string `json:"external_name"`
	ExternalID   *string `json:"external_id"`
	Type         *string `json:"type"`
	Value        *string `json:"value"`
	// ShouldRotate indicates that the user requested the pod secret to be
	// rotated to a new value. This only applies to the project's pod secret.
	ShouldRotate *bool `json:"should_rotate"`
	// RepoCreds, if set, are the new repository credentials to store. This only
	// applies to repository credentials.
	RepoCreds *APIRepositoryCredentials `json:"repo_creds"`
}

type APIRepositoryCredentials struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
}

func (cr *APIContainerSecret) BuildFromService(h model.ContainerSecret) error {
	if h.Type == model.ContainerSecretRepoCreds && h.Value != "" {
		// If the plaintext secret value is available and this secret is a repo
		// cred, the value is the repo creds encoded as JSON, so convert it back
		// to its structured form for the REST API.
		var apiRepoCreds APIRepositoryCredentials
		if err := json.Unmarshal([]byte(h.Value), &apiRepoCreds); err != nil {
			return errors.Wrap(err, "unmarshalling repository credentials")
		}
		cr.RepoCreds = &apiRepoCreds
	}
	if h.Value != "" {
		cr.Value = utility.ToStringPtr(h.Value)
	}
	cr.Name = utility.ToStringPtr(h.Name)
	cr.ExternalID = utility.ToStringPtr(h.ExternalID)
	cr.Type = utility.ToStringPtr(string(h.Type))
	return nil
}

func (cr *APIContainerSecret) ToService() (*model.ContainerSecret, error) {
	secret := model.ContainerSecret{
		Name:       utility.FromStringPtr(cr.Name),
		ExternalID: utility.FromStringPtr(cr.ExternalID),
		Type:       model.ContainerSecretType(utility.FromStringPtr(cr.Type)),
		Value:      utility.FromStringPtr(cr.Value),
	}
	if utility.FromBoolPtr(cr.ShouldRotate) {
		if secret.Type == model.ContainerSecretPodSecret {
			// The user has requested to rotate the pod secret to a new value.
			secret.Value = utility.RandomString()
		} else {
			return nil, errors.Errorf("can only rotate the value for the pod secret, not for container secret '%s' of type '%s'", secret.Name, secret.Type)
		}
	}
	if cr.RepoCreds != nil {
		if secret.Type == model.ContainerSecretRepoCreds {
			// If this is a repo cred and the credentials must be stored, the value
			// to store must be encoded as JSON.
			b, err := json.Marshal(cr.RepoCreds)
			if err != nil {
				return nil, errors.Wrap(err, "marshalling repository credentials as JSON")
			}
			secret.Value = string(b)
		} else {
			return nil, errors.Errorf("can only set credentials for repo creds, not for container secret '%s' of type '%s'", secret.Name, secret.Type)
		}
	}
	return &secret, nil
}

type APIContainerResources struct {
	Name     *string `bson:"name" json:"name"`
	MemoryMB *int    `bson:"memory_mb" json:"memory_mb"`
	CPU      *int    `bson:"cpu" json:"cpu"`
}

func (cr *APIContainerResources) BuildFromService(h model.ContainerResources) {
	cr.Name = utility.ToStringPtr(h.Name)
	cr.MemoryMB = utility.ToIntPtr(h.MemoryMB)
	cr.CPU = utility.ToIntPtr(h.CPU)
}

func (cr *APIContainerResources) ToService() model.ContainerResources {
	return model.ContainerResources{
		Name:     utility.FromStringPtr(cr.Name),
		MemoryMB: utility.FromIntPtr(cr.MemoryMB),
		CPU:      utility.FromIntPtr(cr.CPU),
	}
}

type APIWorkstationSetupCommand struct {
	Command   *string `bson:"command" json:"command"`
	Directory *string `bson:"directory" json:"directory"`
}

func (c *APIWorkstationConfig) ToService() model.WorkstationConfig {
	res := model.WorkstationConfig{}
	res.GitClone = utility.BoolPtrCopy(c.GitClone)
	if c.SetupCommands != nil {
		res.SetupCommands = []model.WorkstationSetupCommand{}
	}
	for _, apiCmd := range c.SetupCommands {
		cmd := model.WorkstationSetupCommand{}
		cmd.Command = utility.FromStringPtr(apiCmd.Command)
		cmd.Directory = utility.FromStringPtr(apiCmd.Directory)
		res.SetupCommands = append(res.SetupCommands, cmd)
	}
	return res
}

func (c *APIWorkstationConfig) BuildFromService(config model.WorkstationConfig) {
	c.GitClone = utility.BoolPtrCopy(config.GitClone)
	if config.SetupCommands != nil {
		c.SetupCommands = []APIWorkstationSetupCommand{}
	}
	for _, cmd := range config.SetupCommands {
		apiCmd := APIWorkstationSetupCommand{}
		apiCmd.Command = utility.ToStringPtr(cmd.Command)
		apiCmd.Directory = utility.ToStringPtr(cmd.Directory)
		c.SetupCommands = append(c.SetupCommands, apiCmd)
	}
}

type APIParameterInfo struct {
	Key         *string `json:"key"`
	Value       *string `json:"value"`
	Description *string `json:"description"`
}

func (c *APIParameterInfo) ToService() model.ParameterInfo {
	res := model.ParameterInfo{}
	res.Key = utility.FromStringPtr(c.Key)
	res.Value = utility.FromStringPtr(c.Value)
	res.Description = utility.FromStringPtr(c.Description)
	return res
}

func (c *APIParameterInfo) BuildFromService(info model.ParameterInfo) {
	c.Key = utility.ToStringPtr(info.Key)
	c.Value = utility.ToStringPtr(info.Value)
	c.Description = utility.ToStringPtr(info.Description)
}

type APIRepositoryErrorDetails struct {
	Exists            *bool   `json:"exists"`
	InvalidRevision   *string `json:"invalid_revision"`
	MergeBaseRevision *string `json:"merge_base_revision"`
}

func (t *APIRepositoryErrorDetails) BuildFromService(h model.RepositoryErrorDetails) {
	t.Exists = utility.ToBoolPtr(h.Exists)
	t.InvalidRevision = utility.ToStringPtr(h.InvalidRevision)
	t.MergeBaseRevision = utility.ToStringPtr(h.MergeBaseRevision)
}

type APIProjectRef struct {
	Id *string `json:"id"`
	// Owner of project repository
	Owner *string `json:"owner_name"`
	// Repository name
	Repo *string `json:"repo_name"`
	// Name of branch
	Branch *string `json:"branch_name"`
	// Whether evergreen is enabled for this project
	Enabled *bool `json:"enabled"`
	// A user must be logged in to view private projects
	Private *bool `json:"private"`
	// Unique identifier of a specific patch
	BatchTime int `json:"batch_time"`
	// Path to config file in repo
	RemotePath          *string `json:"remote_path"`
	SpawnHostScriptPath *string `json:"spawn_host_script_path"`
	// Internal evergreen identifier for project
	Identifier *string `json:"identifier"`
	// Project name displayed to users
	DisplayName *string `json:"display_name"`
	// List of identifiers of tasks used in this patch
	DeactivatePrevious *bool `json:"deactivate_previous"`
	// If true, repotracker is run on github push events. If false, repotracker is run periodically every few minutes.
	TracksPushEvents *bool `json:"tracks_push_events"`
	// Enable github pull request testing
	PRTestingEnabled       *bool   `json:"pr_testing_enabled"`
	ManualPRTestingEnabled *bool   `json:"manual_pr_testing_enabled"`
	GitTagVersionsEnabled  *bool   `json:"git_tag_versions_enabled"`
	GithubChecksEnabled    *bool   `json:"github_checks_enabled"`
	UseRepoSettings        *bool   `json:"use_repo_settings"`
	RepoRefId              *string `json:"repo_ref_id"`
	// Options for commit queue
	CommitQueue            APICommitQueueParams      `json:"commit_queue"`
	TaskSync               APITaskSyncOptions        `json:"task_sync"`
	TaskAnnotationSettings APITaskAnnotationSettings `json:"task_annotation_settings"`
	BuildBaronSettings     APIBuildBaronSettings     `json:"build_baron_settings"`
	PerfEnabled            *bool                     `json:"perf_enabled"`
	Hidden                 *bool                     `json:"hidden"`
	// Disable patching
	PatchingDisabled      *bool                      `json:"patching_disabled"`
	RepotrackerDisabled   *bool                      `json:"repotracker_disabled"`
	RepotrackerError      *APIRepositoryErrorDetails `json:"repotracker_error"`
	DispatchingDisabled   *bool                      `json:"dispatching_disabled"`
	StepbackDisabled      *bool                      `json:"stepback_disabled"`
	StepbackBisect        *bool                      `json:"stepback_bisect"`
	VersionControlEnabled *bool                      `json:"version_control_enabled"`
	DisabledStatsCache    *bool                      `json:"disabled_stats_cache"`
	// Usernames of project admins. Can be null for some projects (EVG-6598).
	Admins []*string `json:"admins"`
	// Usernames of project admins to remove
	DeleteAdmins                []*string `json:"delete_admins,omitempty"`
	GitTagAuthorizedUsers       []*string `json:"git_tag_authorized_users" bson:"git_tag_authorized_users"`
	DeleteGitTagAuthorizedUsers []*string `json:"delete_git_tag_authorized_users,omitempty" bson:"delete_git_tag_authorized_users,omitempty"`
	GitTagAuthorizedTeams       []*string `json:"git_tag_authorized_teams" bson:"git_tag_authorized_teams"`
	DeleteGitTagAuthorizedTeams []*string `json:"delete_git_tag_authorized_teams,omitempty" bson:"delete_git_tag_authorized_teams,omitempty"`
	// Notify original committer (or admins) when build fails
	NotifyOnBuildFailure *bool `json:"notify_on_failure"`
	Restricted           *bool `json:"restricted"`
	// Only used when modifying projects to change the base revision and run the repotracker.
	Revision *string `json:"revision"`
	// a list of triggers for the project
	Triggers             []APITriggerDefinition      `json:"triggers"`
	GithubTriggerAliases []*string                   `json:"github_trigger_aliases"`
	PatchTriggerAliases  []APIPatchTriggerDefinition `json:"patch_trigger_aliases"`
	// a list of aliases for the project
	Aliases []APIProjectAlias `json:"aliases"`
	// project variables information
	Variables         APIProjectVars       `json:"variables"`
	WorkstationConfig APIWorkstationConfig `json:"workstation_config"`
	// a list of subscriptions for the project
	Subscriptions []APISubscription `json:"subscriptions"`
	// subscription IDs. Will delete these subscriptions when given.
	DeleteSubscriptions      []*string                    `json:"delete_subscriptions,omitempty"`
	PeriodicBuilds           []APIPeriodicBuildDefinition `json:"periodic_builds,omitempty"`
	ContainerSizeDefinitions []APIContainerResources      `json:"container_size_definitions"`
	ContainerSecrets         []APIContainerSecret         `json:"container_secrets,omitempty"`
	// DeleteContainerSecrets contains names of container secrets to be deleted.
	DeleteContainerSecrets []string                `json:"delete_container_secrets,omitempty"`
	ExternalLinks          []APIExternalLink       `json:"external_links"`
	Banner                 APIProjectBanner        `json:"banner"`
	ParsleyFilters         []APIParsleyFilter      `json:"parsley_filters"`
	ProjectHealthView      model.ProjectHealthView `json:"project_health_view"`
}

// ToService returns a service layer ProjectRef using the data from APIProjectRef
func (p *APIProjectRef) ToService() (*model.ProjectRef, error) {
	projectRef := model.ProjectRef{
		Owner:                  utility.FromStringPtr(p.Owner),
		Repo:                   utility.FromStringPtr(p.Repo),
		Branch:                 utility.FromStringPtr(p.Branch),
		Enabled:                utility.FromBoolPtr(p.Enabled),
		Private:                utility.BoolPtrCopy(p.Private),
		Restricted:             utility.BoolPtrCopy(p.Restricted),
		BatchTime:              p.BatchTime,
		RemotePath:             utility.FromStringPtr(p.RemotePath),
		Id:                     utility.FromStringPtr(p.Id),
		Identifier:             utility.FromStringPtr(p.Identifier),
		DisplayName:            utility.FromStringPtr(p.DisplayName),
		DeactivatePrevious:     utility.BoolPtrCopy(p.DeactivatePrevious),
		TracksPushEvents:       utility.BoolPtrCopy(p.TracksPushEvents),
		PRTestingEnabled:       utility.BoolPtrCopy(p.PRTestingEnabled),
		ManualPRTestingEnabled: utility.BoolPtrCopy(p.ManualPRTestingEnabled),
		GitTagVersionsEnabled:  utility.BoolPtrCopy(p.GitTagVersionsEnabled),
		GithubChecksEnabled:    utility.BoolPtrCopy(p.GithubChecksEnabled),
		RepoRefId:              utility.FromStringPtr(p.RepoRefId),
		CommitQueue:            p.CommitQueue.ToService(),
		TaskSync:               p.TaskSync.ToService(),
		WorkstationConfig:      p.WorkstationConfig.ToService(),
		BuildBaronSettings:     p.BuildBaronSettings.ToService(),
		TaskAnnotationSettings: p.TaskAnnotationSettings.ToService(),
		PerfEnabled:            utility.BoolPtrCopy(p.PerfEnabled),
		Hidden:                 utility.BoolPtrCopy(p.Hidden),
		PatchingDisabled:       utility.BoolPtrCopy(p.PatchingDisabled),
		RepotrackerDisabled:    utility.BoolPtrCopy(p.RepotrackerDisabled),
		DispatchingDisabled:    utility.BoolPtrCopy(p.DispatchingDisabled),
		StepbackDisabled:       utility.BoolPtrCopy(p.StepbackDisabled),
		StepbackBisect:         utility.BoolPtrCopy(p.StepbackBisect),
		VersionControlEnabled:  utility.BoolPtrCopy(p.VersionControlEnabled),
		DisabledStatsCache:     utility.BoolPtrCopy(p.DisabledStatsCache),
		NotifyOnBuildFailure:   utility.BoolPtrCopy(p.NotifyOnBuildFailure),
		SpawnHostScriptPath:    utility.FromStringPtr(p.SpawnHostScriptPath),
		Admins:                 utility.FromStringPtrSlice(p.Admins),
		GitTagAuthorizedUsers:  utility.FromStringPtrSlice(p.GitTagAuthorizedUsers),
		GitTagAuthorizedTeams:  utility.FromStringPtrSlice(p.GitTagAuthorizedTeams),
		GithubTriggerAliases:   utility.FromStringPtrSlice(p.GithubTriggerAliases),
		Banner:                 p.Banner.ToService(),
		ProjectHealthView:      p.ProjectHealthView,
	}

	if projectRef.ProjectHealthView == "" {
		projectRef.ProjectHealthView = model.ProjectHealthViewFailed
	}

	// Copy triggers
	if p.Triggers != nil {
		triggers := []model.TriggerDefinition{}
		for _, t := range p.Triggers {
			triggers = append(triggers, t.ToService())
		}
		projectRef.Triggers = triggers
	}

	// Copy periodic builds
	if p.PeriodicBuilds != nil {
		builds := []model.PeriodicBuildDefinition{}
		for _, b := range p.PeriodicBuilds {
			builds = append(builds, b.ToService())
		}
		projectRef.PeriodicBuilds = builds
	}

	// Copy External Links
	if p.ExternalLinks != nil {
		links := []model.ExternalLink{}
		for _, l := range p.ExternalLinks {
			links = append(links, l.ToService())
		}
		projectRef.ExternalLinks = links
	}

	// Copy Parsley filters
	if p.ParsleyFilters != nil {
		parsleyFilters := []model.ParsleyFilter{}
		for _, f := range p.ParsleyFilters {
			parsleyFilters = append(parsleyFilters, f.ToService())
		}
		projectRef.ParsleyFilters = parsleyFilters
	}

	if p.PatchTriggerAliases != nil {
		patchTriggers := []patch.PatchTriggerDefinition{}
		for _, a := range p.PatchTriggerAliases {
			patchTriggers = append(patchTriggers, a.ToService())
		}
		projectRef.PatchTriggerAliases = patchTriggers
	}

	for _, size := range p.ContainerSizeDefinitions {
		projectRef.ContainerSizeDefinitions = append(projectRef.ContainerSizeDefinitions, size.ToService())
	}

	for idx, secret := range p.ContainerSecrets {
		if utility.StringSliceContains(p.DeleteContainerSecrets, utility.FromStringPtr(secret.Name)) {
			continue
		}

		apiContainerSecret, err := secret.ToService()
		if err != nil {
			return nil, errors.Wrapf(err, "converting container secret at index %d to service model", idx)
		}
		projectRef.ContainerSecrets = append(projectRef.ContainerSecrets, *apiContainerSecret)
	}

	return &projectRef, nil
}

// BuildPublicFields only builds the fields that anyone should be able to see
// so that we can return these to non project admins.
func (p *APIProjectRef) BuildPublicFields(projectRef model.ProjectRef) error {
	p.Id = utility.ToStringPtr(projectRef.Id)
	p.Identifier = utility.ToStringPtr(projectRef.Identifier)
	p.DisplayName = utility.ToStringPtr(projectRef.DisplayName)
	p.Owner = utility.ToStringPtr(projectRef.Owner)
	p.Repo = utility.ToStringPtr(projectRef.Repo)
	p.Branch = utility.ToStringPtr(projectRef.Branch)
	p.Enabled = utility.ToBoolPtr(projectRef.Enabled)
	p.Admins = utility.ToStringPtrSlice(projectRef.Admins)
	p.Private = utility.BoolPtrCopy(projectRef.Private)
	p.Restricted = utility.BoolPtrCopy(projectRef.Restricted)
	p.BatchTime = projectRef.BatchTime
	p.RemotePath = utility.ToStringPtr(projectRef.RemotePath)
	p.DeactivatePrevious = projectRef.DeactivatePrevious
	p.TracksPushEvents = utility.BoolPtrCopy(projectRef.TracksPushEvents)
	p.PRTestingEnabled = utility.BoolPtrCopy(projectRef.PRTestingEnabled)
	p.ManualPRTestingEnabled = utility.BoolPtrCopy(projectRef.ManualPRTestingEnabled)
	p.GitTagVersionsEnabled = utility.BoolPtrCopy(projectRef.GitTagVersionsEnabled)
	p.GithubChecksEnabled = utility.BoolPtrCopy(projectRef.GithubChecksEnabled)
	p.UseRepoSettings = utility.ToBoolPtr(projectRef.UseRepoSettings())
	p.RepoRefId = utility.ToStringPtr(projectRef.RepoRefId)
	p.PerfEnabled = utility.BoolPtrCopy(projectRef.PerfEnabled)
	p.Hidden = utility.BoolPtrCopy(projectRef.Hidden)
	p.PatchingDisabled = utility.BoolPtrCopy(projectRef.PatchingDisabled)
	p.RepotrackerDisabled = utility.BoolPtrCopy(projectRef.RepotrackerDisabled)
	p.DispatchingDisabled = utility.BoolPtrCopy(projectRef.DispatchingDisabled)
	p.StepbackDisabled = utility.BoolPtrCopy(projectRef.StepbackDisabled)
	p.StepbackBisect = utility.BoolPtrCopy(projectRef.StepbackBisect)
	p.VersionControlEnabled = utility.BoolPtrCopy(projectRef.VersionControlEnabled)
	p.DisabledStatsCache = utility.BoolPtrCopy(projectRef.DisabledStatsCache)
	p.NotifyOnBuildFailure = utility.BoolPtrCopy(projectRef.NotifyOnBuildFailure)
	p.SpawnHostScriptPath = utility.ToStringPtr(projectRef.SpawnHostScriptPath)
	p.GitTagAuthorizedUsers = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedUsers)
	p.GitTagAuthorizedTeams = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedTeams)
	p.GithubTriggerAliases = utility.ToStringPtrSlice(projectRef.GithubTriggerAliases)

	if projectRef.ProjectHealthView == "" {
		projectRef.ProjectHealthView = model.ProjectHealthViewFailed
	}
	p.ProjectHealthView = projectRef.ProjectHealthView

	cq := APICommitQueueParams{}
	cq.BuildFromService(projectRef.CommitQueue)
	p.CommitQueue = cq

	var taskSync APITaskSyncOptions
	taskSync.BuildFromService(projectRef.TaskSync)
	p.TaskSync = taskSync

	buildbaronConfig := APIBuildBaronSettings{}
	buildbaronConfig.BuildFromService(projectRef.BuildBaronSettings)
	p.BuildBaronSettings = buildbaronConfig

	projectBanner := APIProjectBanner{}
	projectBanner.BuildFromService(projectRef.Banner)
	p.Banner = projectBanner

	if projectRef.RepotrackerError != nil {
		repotrackerErr := APIRepositoryErrorDetails{}
		repotrackerErr.BuildFromService(*projectRef.RepotrackerError)
		p.RepotrackerError = &repotrackerErr
	}

	// Copy triggers
	if projectRef.Triggers != nil {
		triggers := []APITriggerDefinition{}
		for _, t := range projectRef.Triggers {
			apiTrigger := APITriggerDefinition{}
			apiTrigger.BuildFromService(t)
			triggers = append(triggers, apiTrigger)
		}
		p.Triggers = triggers
	}

	// copy periodic builds
	if projectRef.PeriodicBuilds != nil {
		periodicBuilds := []APIPeriodicBuildDefinition{}
		for _, pb := range projectRef.PeriodicBuilds {
			periodicBuild := APIPeriodicBuildDefinition{}
			periodicBuild.BuildFromService(pb)
			periodicBuilds = append(periodicBuilds, periodicBuild)
		}
		p.PeriodicBuilds = periodicBuilds
	}

	if projectRef.PatchTriggerAliases != nil {
		patchTriggers := []APIPatchTriggerDefinition{}
		for idx, a := range projectRef.PatchTriggerAliases {
			trigger := APIPatchTriggerDefinition{}
			if err := trigger.BuildFromService(a); err != nil {
				return errors.Wrapf(err, "converting patch trigger alias at index %d to service model", idx)
			}
			patchTriggers = append(patchTriggers, trigger)
		}
		p.PatchTriggerAliases = patchTriggers
	}

	for _, size := range projectRef.ContainerSizeDefinitions {
		var apiSize APIContainerResources
		apiSize.BuildFromService(size)
		p.ContainerSizeDefinitions = append(p.ContainerSizeDefinitions, apiSize)
	}

	// copy external links
	if projectRef.ExternalLinks != nil {
		externalLinks := []APIExternalLink{}
		for _, l := range projectRef.ExternalLinks {
			externalLink := APIExternalLink{}
			externalLink.BuildFromService(l)
			externalLinks = append(externalLinks, externalLink)
		}
		p.ExternalLinks = externalLinks
	}

	// Copy Parsley filters
	if projectRef.ParsleyFilters != nil {
		parsleyFilters := []APIParsleyFilter{}
		for _, f := range projectRef.ParsleyFilters {
			parsleyFilter := APIParsleyFilter{}
			parsleyFilter.BuildFromService(f)
			parsleyFilters = append(parsleyFilters, parsleyFilter)
		}
		p.ParsleyFilters = parsleyFilters
	}

	return nil
}

func (p *APIProjectRef) BuildFromService(projectRef model.ProjectRef) error {
	if err := p.BuildPublicFields(projectRef); err != nil {
		return err
	}

	taskAnnotationConfig := APITaskAnnotationSettings{}
	taskAnnotationConfig.BuildFromService(projectRef.TaskAnnotationSettings)
	p.TaskAnnotationSettings = taskAnnotationConfig

	workstationConfig := APIWorkstationConfig{}
	workstationConfig.BuildFromService(projectRef.WorkstationConfig)
	p.WorkstationConfig = workstationConfig

	for idx, secret := range projectRef.ContainerSecrets {
		var apiSecret APIContainerSecret
		if err := apiSecret.BuildFromService(secret); err != nil {
			return errors.Wrapf(err, "converting container secret at index %d to service model", idx)
		}
		p.ContainerSecrets = append(p.ContainerSecrets, apiSecret)
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
		grip.Error(recovery.HandlePanicWithError(recover(), err, fmt.Sprintf("panicked while recursively defaulting booleans for field number %d", i)))
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
