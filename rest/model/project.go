package model

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
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
	// Identifier of project to watch.
	Project *string `json:"project"`
	// Trigger on build, task, or push.
	Level *string `json:"level"`
	// Identifier for the definition.
	DefinitionID *string `json:"definition_id"`
	// Build variant regex to match.
	BuildVariantRegex *string `json:"variant_regex"`
	// Task regex to match.
	TaskRegex *string `json:"task_regex"`
	// Task status to trigger for (or "*" for all).
	Status *string `json:"status"`
	// Number of days after commit when the trigger cannot run.
	DateCutoff *int `json:"date_cutoff"`
	// Project configuration file for the trigger.
	ConfigFile *string `json:"config_file"`
	// Alias to run for the trigger.
	Alias *string `json:"alias"`
	// Deactivate downstream versions created by this trigger.
	UnscheduleDownstreamVersions *bool `json:"unschedule_downstream_versions"`
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
	// Alias to run in the downstream project.
	Alias *string `json:"alias"`
	// ID of the downstream project.
	ChildProjectId *string `json:"child_project_id"`
	// Identifier of the downstream project.
	ChildProjectIdentifier *string `json:"child_project_identifier"`
	// List of task specifiers.
	TaskSpecifiers []APITaskSpecifier `json:"task_specifiers"`
	// Status for the parent patch to conditionally kick off the child patch.
	Status *string `json:"status,omitempty"`
	// Name of the module corresponding to the upstream project in the
	// downstream project's YAML.
	ParentAsModule *string `json:"parent_as_module,omitempty"`
	// An optional field representing the revision at which to create the downstream patch.
	// By default, this field is empty and the downstream patch will be based off of its
	// most recent commit.
	DownstreamRevision *string `json:"downstream_revision,omitempty"`
	// The list of variants/tasks from the alias that will run in the downstream
	// project.
	VariantsTasks []VariantTask `json:"variants_tasks,omitempty"`
}

func (t *APIPatchTriggerDefinition) BuildFromService(ctx context.Context, def patch.PatchTriggerDefinition) error {
	t.ChildProjectId = utility.ToStringPtr(def.ChildProject) // we store the real ID in the child project field
	identifier, err := model.GetIdentifierForProject(ctx, def.ChildProject)
	if err != nil {
		return errors.Wrapf(err, "getting identifier for child project '%s'", def.ChildProject)
	}
	t.ChildProjectIdentifier = utility.ToStringPtr(identifier)
	// not sure in which direction this should go
	t.Alias = utility.ToStringPtr(def.Alias)
	t.Status = utility.ToStringPtr(def.Status)
	t.DownstreamRevision = utility.ToStringPtr(def.DownstreamRevision)
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
	trigger.DownstreamRevision = utility.FromStringPtr(t.DownstreamRevision)
	var specifiers []patch.TaskSpecifier
	for _, ts := range t.TaskSpecifiers {
		specifiers = append(specifiers, ts.ToService())
	}
	trigger.TaskSpecifiers = specifiers
	return trigger
}

type APITaskSpecifier struct {
	// Patch alias to run.
	PatchAlias *string `json:"patch_alias,omitempty"`
	// Regex matching tasks to run.
	TaskRegex *string `json:"task_regex,omitempty"`
	// Regex matching build variants to run.
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
	// Identifier for the periodic build.
	ID *string `json:"id"`
	// Project config file to use for the periodic build.
	ConfigFile *string `json:"config_file"`
	// Interval (in hours) between periodic build runs.
	IntervalHours *int `json:"interval_hours"`
	// Cron specification for when to run periodic builds.
	Cron *string `json:"cron"`
	// Alias to run for the periodic build.
	Alias *string `json:"alias,omitempty"`
	// Message to display in the version metadata.
	Message *string `json:"message,omitempty"`
	// Next time that the periodic build will run.
	NextRunTime *time.Time `json:"next_run_time,omitempty"`
}

type APIExternalLink struct {
	// Display name for the URL.
	DisplayName *string `json:"display_name"`
	// Requester filter for when to display the link.
	Requesters []*string `json:"requesters"`
	// URL format to add to the version metadata panel.
	URLTemplate *string `json:"url_template"`
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
	// Banner theme.
	Theme evergreen.BannerTheme `json:"theme"`
	// Banner text.
	Text *string `json:"text"`
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
	// Enable/disable the commit queue.
	Enabled *bool `json:"enabled"`
	// Method of merging (squash, merge, or rebase).
	MergeMethod *string `json:"merge_method"`
	// Message to display when users interact with the commit queue.
	Message *string `json:"message"`
}

func (cqParams *APICommitQueueParams) BuildFromService(params model.CommitQueueParams) {
	cqParams.Enabled = utility.BoolPtrCopy(params.Enabled)
	cqParams.MergeMethod = utility.ToStringPtr(params.MergeMethod)
	cqParams.Message = utility.ToStringPtr(params.Message)
}

func (cqParams *APICommitQueueParams) ToService() model.CommitQueueParams {
	serviceParams := model.CommitQueueParams{}
	serviceParams.Enabled = utility.BoolPtrCopy(cqParams.Enabled)
	serviceParams.MergeMethod = utility.FromStringPtr(cqParams.MergeMethod)
	serviceParams.Message = utility.FromStringPtr(cqParams.Message)

	return serviceParams
}

type APIBuildBaronSettings struct {
	// Jira project where tickets should be created.
	TicketCreateProject *string `bson:"ticket_create_project" json:"ticket_create_project"`
	// Type of ticket to create.
	TicketCreateIssueType *string `bson:"ticket_create_issue_type" json:"ticket_create_issue_type"`
	// Jira project to search for tickets.
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
	// Options for webhooks.
	FileTicketWebhook APIWebHook `bson:"web_hook" json:"web_hook"`
}

type APIWebHook struct {
	// Webhook endpoint
	Endpoint *string `bson:"endpoint" json:"endpoint"`
	// Webhook secret
	Secret *string `bson:"secret" json:"secret"`
}

func (ta *APITaskAnnotationSettings) ToService() evergreen.AnnotationsSettings {
	res := evergreen.AnnotationsSettings{}
	webhook := evergreen.WebHook{}
	webhook.Secret = utility.FromStringPtr(ta.FileTicketWebhook.Secret)
	webhook.Endpoint = utility.FromStringPtr(ta.FileTicketWebhook.Endpoint)
	res.FileTicketWebhook = webhook
	return res
}

func (ta *APITaskAnnotationSettings) BuildFromService(config evergreen.AnnotationsSettings) {
	apiWebhook := APIWebHook{}
	apiWebhook.Secret = utility.ToStringPtr(config.FileTicketWebhook.Secret)
	apiWebhook.Endpoint = utility.ToStringPtr(config.FileTicketWebhook.Endpoint)
	ta.FileTicketWebhook = apiWebhook
}

type APIWorkstationConfig struct {
	// List of setup commands to run.
	SetupCommands []APIWorkstationSetupCommand `bson:"setup_commands" json:"setup_commands"`
	// Git clone the project in the workstation.
	GitClone *bool `bson:"git_clone" json:"git_clone"`
}

type APIRepositoryCredentials struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
}

type APIWorkstationSetupCommand struct {
	// Command to run in the workstation.
	Command *string `bson:"command" json:"command"`
	// Directory where the command runs.
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

type APIGitHubDynamicTokenPermissionGroup struct {
	// Name of the GitHub permission group.
	Name *string `json:"name"`
	// Permissions for the GitHub permission group.
	Permissions map[string]string `json:"permissions"`
	// AllPermissions is a flag that indicates that the group has all permissions.
	// If this is set to true, the Permissions field is ignored.
	// If this is set to false, the Permissions field is used (and may be
	// nil, representing no permissions).
	AllPermissions *bool `json:"all_permissions"`
}

func (p *APIGitHubDynamicTokenPermissionGroup) ToService() (model.GitHubDynamicTokenPermissionGroup, error) {
	group := model.GitHubDynamicTokenPermissionGroup{
		Name: utility.FromStringPtr(p.Name),
	}
	metadata := mapstructure.Metadata{}
	// The github.InstallationPermissions struct has json struct tags that we can
	// latch on to for decoding the permissions.
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:     "json",
		Result:      &group.Permissions,
		ErrorUnused: true,
		Metadata:    &metadata,
	})
	if err != nil {
		return group, errors.Wrap(err, "creating decoder for GitHub permissions")
	}
	err = decoder.Decode(p.Permissions)
	if err != nil {
		return group, errors.Wrap(err, "decoding GitHub permissions")
	}
	// If the 'Permissions' map is empty and 'AllPermissions' is true, the group
	// has all permissions.
	group.AllPermissions = utility.FromBoolPtr(p.AllPermissions)
	if group.AllPermissions && len(metadata.Keys) > 0 {
		return group, errors.New("a group will all permissions must have no permissions set")
	}
	return group, nil
}

func (p *APIGitHubDynamicTokenPermissionGroup) BuildFromService(h model.GitHubDynamicTokenPermissionGroup) error {
	p.Name = utility.ToStringPtr(h.Name)

	permissions := map[string]string{}
	data, err := json.Marshal(h.Permissions)
	if err != nil {
		return errors.Wrapf(err, "converting GitHub permission group '%s'", h.Name)
	}
	if err := json.Unmarshal(data, &permissions); err != nil {
		return errors.Wrap(err, "unmarshalling GitHub permissions")
	}
	p.Permissions = permissions

	p.AllPermissions = utility.ToBoolPtr(h.AllPermissions)

	return nil
}

type APITestSelectionSettings struct {
	// Whether or not test selection features can be used.
	Allowed *bool `json:"allowed,omitzero"`
	// Whether or not test selection is enabled by default for tasks.
	DefaultEnabled *bool `json:"default_enabled,omitzero"`
}

func (ts *APITestSelectionSettings) ToService() model.TestSelectionSettings {
	return model.TestSelectionSettings{
		Allowed:        utility.BoolPtrCopy(ts.Allowed),
		DefaultEnabled: utility.BoolPtrCopy(ts.DefaultEnabled),
	}
}

func (ts *APITestSelectionSettings) BuildFromService(settings model.TestSelectionSettings) {
	ts.Allowed = utility.BoolPtrCopy(settings.Allowed)
	ts.DefaultEnabled = utility.BoolPtrCopy(settings.DefaultEnabled)
}

type APIProjectRef struct {
	Id *string `json:"id"`
	// GitHub org name.
	Owner *string `json:"owner_name"`
	// GitHub repository name.
	Repo *string `json:"repo_name"`
	// Name of tracking branch.
	Branch *string `json:"branch_name"`
	// Whether evergreen is enabled for this project.
	Enabled *bool `json:"enabled"`
	// Time interval between commits for Evergreen to activate.
	BatchTime int `json:"batch_time"`
	// Path to config file in repo.
	RemotePath *string `json:"remote_path"`
	// Oldest allowed merge base for PR patches
	OldestAllowedMergeBase *string `json:"oldest_allowed_merge_base"`
	// File path to script that users can run on spawn hosts loaded with task
	// data.
	SpawnHostScriptPath *string `json:"spawn_host_script_path"`
	// Internal evergreen identifier for project.
	Identifier *string `json:"identifier"`
	// Project name displayed to users.
	DisplayName *string `json:"display_name"`
	// List of identifiers of tasks used in this patch.
	DeactivatePrevious *bool `json:"deactivate_previous"`
	// If true, repotracker is run on github push events. If false, repotracker is run periodically every few minutes.
	TracksPushEvents *bool `json:"tracks_push_events"`
	// Enable GitHub automated pull request testing.
	PRTestingEnabled *bool `json:"pr_testing_enabled"`
	// Enable GitHub manual pull request testing.
	ManualPRTestingEnabled *bool `json:"manual_pr_testing_enabled"`
	// Enable testing when git tags are pushed.
	GitTagVersionsEnabled *bool `json:"git_tag_versions_enabled"`
	// Enable GitHub checks.
	GithubChecksEnabled *bool `json:"github_checks_enabled"`
	// Whether or not to default to using repo settings.
	UseRepoSettings *bool `json:"use_repo_settings"`
	// Identifier of the attached repo ref. Cannot be modified by users.
	RepoRefId *string `json:"repo_ref_id"`
	// Options for commit queue.
	CommitQueue APICommitQueueParams `json:"commit_queue"`
	// Options for task annotations.
	TaskAnnotationSettings APITaskAnnotationSettings `json:"task_annotation_settings"`
	// Options for Build Baron.
	BuildBaronSettings APIBuildBaronSettings `json:"build_baron_settings"`
	// Enable the performance plugin.
	PerfEnabled *bool `json:"perf_enabled"`
	// Whether or not the project can be seen in the UI. Cannot be modified by
	// users.
	Hidden *bool `json:"hidden"`
	// Disable patching.
	PatchingDisabled *bool `json:"patching_disabled"`
	// Disable the repotracker.
	RepotrackerDisabled *bool `json:"repotracker_disabled"`
	// Error from the repotracker, if any. Cannot be modified by users.
	RepotrackerError *APIRepositoryErrorDetails `json:"repotracker_error"`
	// Disable task dispatching.
	DispatchingDisabled *bool `json:"dispatching_disabled"`
	// Disable stepback.
	StepbackDisabled *bool `json:"stepback_disabled"`
	// Enable debug spawn host functionality.
	DebugSpawnHostsDisabled *bool `json:"debug_spawn_hosts_disabled"`
	// Use bisect stepback instead of linear.
	StepbackBisect *bool `json:"stepback_bisect"`
	// Enable setting project aliases from version-controlled project configs.
	VersionControlEnabled *bool `json:"version_control_enabled"`
	// Disable stats caching.
	DisabledStatsCache *bool `json:"disabled_stats_cache"`
	// Usernames of project admins. Can be null for some projects (EVG-6598).
	Admins []*string `json:"admins"`
	// Usernames of project admins to remove.
	DeleteAdmins []*string `json:"delete_admins,omitempty"`
	// Usernames authorized to submit git tag versions.
	GitTagAuthorizedUsers []*string `json:"git_tag_authorized_users" bson:"git_tag_authorized_users"`
	// Usernames of git tag-authorized users to remove.
	DeleteGitTagAuthorizedUsers []*string `json:"delete_git_tag_authorized_users,omitempty" bson:"delete_git_tag_authorized_users,omitempty"`
	// Names of GitHub teams authorized to submit git tag versions.
	GitTagAuthorizedTeams []*string `json:"git_tag_authorized_teams" bson:"git_tag_authorized_teams"`
	// Names of GitHub teams authorized to submit git tag versions to remove.
	DeleteGitTagAuthorizedTeams []*string `json:"delete_git_tag_authorized_teams,omitempty" bson:"delete_git_tag_authorized_teams,omitempty"`
	// Notify original committer (or admins) when build fails.
	NotifyOnBuildFailure *bool `json:"notify_on_failure"`
	// Prevent users from being able to view this project unless explicitly
	// granted access.
	Restricted *bool `json:"restricted"`
	// Only used when modifying projects to change the base revision and run the repotracker.
	Revision *string `json:"revision"`
	// List of triggers for the project.
	Triggers []APITriggerDefinition `json:"triggers"`
	// List of GitHub pull request trigger aliases.
	GithubPRTriggerAliases []*string `json:"github_trigger_aliases"`
	// List of GitHub merge queue trigger aliases.
	GithubMQTriggerAliases []*string `json:"github_merge_queue_trigger_aliases"`
	// List of patch trigger aliases.
	PatchTriggerAliases []APIPatchTriggerDefinition `json:"patch_trigger_aliases"`
	// List of aliases for the project.
	Aliases []APIProjectAlias `json:"aliases"`
	// Project variables information
	Variables APIProjectVars `json:"variables"`
	// Options for workstations.
	WorkstationConfig APIWorkstationConfig `json:"workstation_config"`
	// List of subscriptions for the project.
	Subscriptions []APISubscription `json:"subscriptions"`
	// IDs of subscriptions to delete.
	DeleteSubscriptions []*string `json:"delete_subscriptions,omitempty"`
	// List of periodic build definitions.
	PeriodicBuilds []APIPeriodicBuildDefinition `json:"periodic_builds,omitempty"`
	// List of external links in the version metadata.
	ExternalLinks []APIExternalLink `json:"external_links"`
	// Options for banner to display for the project.
	Banner APIProjectBanner `json:"banner"`
	// List of custom Parsley filters.
	ParsleyFilters []APIParsleyFilter `json:"parsley_filters"`
	// Default project health view.
	ProjectHealthView model.ProjectHealthView `json:"project_health_view"`
	// List of GitHub permission groups.
	GitHubDynamicTokenPermissionGroups []APIGitHubDynamicTokenPermissionGroup `json:"github_dynamic_token_permission_groups,omitempty"`
	// GitHub permission group by requester.
	GitHubPermissionGroupByRequester map[string]string `json:"github_permission_group_by_requester,omitempty"`
	// Test selection settings.
	TestSelection APITestSelectionSettings `json:"test_selection,omitzero"`
	// Whether or not to run every mainline commit version.
	RunEveryMainlineCommit *bool `json:"run_every_mainline_commit,omitzero"`
}

// ToService returns a service layer ProjectRef using the data from APIProjectRef
func (p *APIProjectRef) ToService() (*model.ProjectRef, error) {
	projectRef := model.ProjectRef{
		Owner:                            utility.FromStringPtr(p.Owner),
		Repo:                             utility.FromStringPtr(p.Repo),
		Branch:                           utility.FromStringPtr(p.Branch),
		Enabled:                          utility.FromBoolPtr(p.Enabled),
		Restricted:                       utility.BoolPtrCopy(p.Restricted),
		BatchTime:                        p.BatchTime,
		RemotePath:                       utility.FromStringPtr(p.RemotePath),
		Id:                               utility.FromStringPtr(p.Id),
		Identifier:                       utility.FromStringPtr(p.Identifier),
		DisplayName:                      utility.FromStringPtr(p.DisplayName),
		DeactivatePrevious:               utility.BoolPtrCopy(p.DeactivatePrevious),
		TracksPushEvents:                 utility.BoolPtrCopy(p.TracksPushEvents),
		PRTestingEnabled:                 utility.BoolPtrCopy(p.PRTestingEnabled),
		ManualPRTestingEnabled:           utility.BoolPtrCopy(p.ManualPRTestingEnabled),
		GitTagVersionsEnabled:            utility.BoolPtrCopy(p.GitTagVersionsEnabled),
		GithubChecksEnabled:              utility.BoolPtrCopy(p.GithubChecksEnabled),
		RepoRefId:                        utility.FromStringPtr(p.RepoRefId),
		CommitQueue:                      p.CommitQueue.ToService(),
		WorkstationConfig:                p.WorkstationConfig.ToService(),
		BuildBaronSettings:               p.BuildBaronSettings.ToService(),
		TaskAnnotationSettings:           p.TaskAnnotationSettings.ToService(),
		PerfEnabled:                      utility.BoolPtrCopy(p.PerfEnabled),
		Hidden:                           utility.BoolPtrCopy(p.Hidden),
		PatchingDisabled:                 utility.BoolPtrCopy(p.PatchingDisabled),
		RepotrackerDisabled:              utility.BoolPtrCopy(p.RepotrackerDisabled),
		DispatchingDisabled:              utility.BoolPtrCopy(p.DispatchingDisabled),
		StepbackDisabled:                 utility.BoolPtrCopy(p.StepbackDisabled),
		StepbackBisect:                   utility.BoolPtrCopy(p.StepbackBisect),
		VersionControlEnabled:            utility.BoolPtrCopy(p.VersionControlEnabled),
		DisabledStatsCache:               utility.BoolPtrCopy(p.DisabledStatsCache),
		NotifyOnBuildFailure:             utility.BoolPtrCopy(p.NotifyOnBuildFailure),
		DebugSpawnHostsDisabled:          utility.BoolPtrCopy(p.DebugSpawnHostsDisabled),
		SpawnHostScriptPath:              utility.FromStringPtr(p.SpawnHostScriptPath),
		OldestAllowedMergeBase:           utility.FromStringPtr(p.OldestAllowedMergeBase),
		Admins:                           utility.FromStringPtrSlice(p.Admins),
		GitTagAuthorizedUsers:            utility.FromStringPtrSlice(p.GitTagAuthorizedUsers),
		GitTagAuthorizedTeams:            utility.FromStringPtrSlice(p.GitTagAuthorizedTeams),
		GithubPRTriggerAliases:           utility.FromStringPtrSlice(p.GithubPRTriggerAliases),
		GithubMQTriggerAliases:           utility.FromStringPtrSlice(p.GithubMQTriggerAliases),
		Banner:                           p.Banner.ToService(),
		ProjectHealthView:                p.ProjectHealthView,
		GitHubPermissionGroupByRequester: p.GitHubPermissionGroupByRequester,
		TestSelection:                    p.TestSelection.ToService(),
		RunEveryMainlineCommit:           utility.FromBoolPtr(p.RunEveryMainlineCommit),
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
		parsleyFilters := []parsley.Filter{}
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

	if p.GitHubDynamicTokenPermissionGroups != nil {
		permissionGroups := []model.GitHubDynamicTokenPermissionGroup{}
		for _, pg := range p.GitHubDynamicTokenPermissionGroups {
			serviceGroup, err := pg.ToService()
			if err != nil {
				return nil, errors.Wrapf(err, "converting GitHub permission group '%s'", utility.FromStringPtr(pg.Name))
			}
			permissionGroups = append(permissionGroups, serviceGroup)
		}
		projectRef.GitHubDynamicTokenPermissionGroups = permissionGroups
	}
	return &projectRef, nil
}

// BuildPublicFields only builds the fields that anyone should be able to see
// so that we can return these to non project admins.
func (p *APIProjectRef) BuildPublicFields(ctx context.Context, projectRef model.ProjectRef) error {
	p.Id = utility.ToStringPtr(projectRef.Id)
	p.Identifier = utility.ToStringPtr(projectRef.Identifier)
	p.DisplayName = utility.ToStringPtr(projectRef.DisplayName)
	p.Owner = utility.ToStringPtr(projectRef.Owner)
	p.Repo = utility.ToStringPtr(projectRef.Repo)
	p.Branch = utility.ToStringPtr(projectRef.Branch)
	p.Enabled = utility.ToBoolPtr(projectRef.Enabled)
	p.Admins = utility.ToStringPtrSlice(projectRef.Admins)
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
	p.DebugSpawnHostsDisabled = utility.BoolPtrCopy(projectRef.DebugSpawnHostsDisabled)
	p.NotifyOnBuildFailure = utility.BoolPtrCopy(projectRef.NotifyOnBuildFailure)
	p.SpawnHostScriptPath = utility.ToStringPtr(projectRef.SpawnHostScriptPath)
	p.OldestAllowedMergeBase = utility.ToStringPtr(projectRef.OldestAllowedMergeBase)
	p.GitTagAuthorizedUsers = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedUsers)
	p.GitTagAuthorizedTeams = utility.ToStringPtrSlice(projectRef.GitTagAuthorizedTeams)
	p.GithubPRTriggerAliases = utility.ToStringPtrSlice(projectRef.GithubPRTriggerAliases)
	p.GithubMQTriggerAliases = utility.ToStringPtrSlice(projectRef.GithubMQTriggerAliases)
	p.GitHubPermissionGroupByRequester = projectRef.GitHubPermissionGroupByRequester
	p.TestSelection.BuildFromService(projectRef.TestSelection)
	p.RunEveryMainlineCommit = utility.ToBoolPtr(projectRef.RunEveryMainlineCommit)

	if projectRef.ProjectHealthView == "" {
		projectRef.ProjectHealthView = model.ProjectHealthViewFailed
	}
	p.ProjectHealthView = projectRef.ProjectHealthView

	cq := APICommitQueueParams{}
	cq.BuildFromService(projectRef.CommitQueue)
	p.CommitQueue = cq

	buildbaronConfig := APIBuildBaronSettings{}
	buildbaronConfig.BuildFromService(projectRef.BuildBaronSettings)
	p.BuildBaronSettings = buildbaronConfig

	projectBanner := APIProjectBanner{}
	projectBanner.BuildFromService(projectRef.Banner)
	p.Banner = projectBanner

	if projectRef.GitHubDynamicTokenPermissionGroups != nil {
		permissionGroups := []APIGitHubDynamicTokenPermissionGroup{}
		for _, pg := range projectRef.GitHubDynamicTokenPermissionGroups {
			apiGroup := APIGitHubDynamicTokenPermissionGroup{}
			if err := apiGroup.BuildFromService(pg); err != nil {
				return errors.Wrapf(err, "converting GitHub permission group '%s' to API model", pg.Name)
			}
			permissionGroups = append(permissionGroups, apiGroup)
		}
		p.GitHubDynamicTokenPermissionGroups = permissionGroups
	}

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
			if err := trigger.BuildFromService(ctx, a); err != nil {
				return errors.Wrapf(err, "converting patch trigger alias at index %d to service model", idx)
			}
			patchTriggers = append(patchTriggers, trigger)
		}
		p.PatchTriggerAliases = patchTriggers
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

func (p *APIProjectRef) BuildFromService(ctx context.Context, projectRef model.ProjectRef) error {
	if err := p.BuildPublicFields(ctx, projectRef); err != nil {
		return err
	}

	taskAnnotationConfig := APITaskAnnotationSettings{}
	taskAnnotationConfig.BuildFromService(projectRef.TaskAnnotationSettings)
	p.TaskAnnotationSettings = taskAnnotationConfig

	workstationConfig := APIWorkstationConfig{}
	workstationConfig.BuildFromService(projectRef.WorkstationConfig)
	p.WorkstationConfig = workstationConfig

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

// CopyProjectOpts is input for the data.CopyProject function.
//
// This options struct doesn't really belong in this package according to
// Evergreen's conventions, and should instead go in rest/data. Unfortunately,
// it has to be here as a necessary hack for the GraphQL generated models.
// gqlgen introduced a breaking change in v0.17.50 that codegenerates ambiguous
// code that initializes a zero value variable called data, and that variable
// declaration shadows the rest/data import package name.
type CopyProjectOpts struct {
	ProjectIdToCopy      string
	NewProjectIdentifier string
	NewProjectId         string
}

type GetProjectTasksOpts struct {
	Limit        int      `json:"num_versions"`
	BuildVariant string   `json:"build_variant"`
	StartAt      int      `json:"start_at"`
	Requesters   []string `json:"requesters"`
}
