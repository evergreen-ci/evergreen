package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

// The ProjectRef struct contains general information, independent of any revision control system, needed to track a given project.
// Booleans that can be defined from both the repo and branch must be pointers, so that branch configurations can specify when to default to the repo.
type ProjectRef struct {
	// Id is the unmodifiable unique ID for the configuration, used internally.
	Id string `bson:"_id" json:"id" yaml:"id"`
	// Identifier must be unique, but is modifiable. Used by users.
	Identifier string `bson:"identifier" json:"identifier" yaml:"identifier"`

	DisplayName          string              `bson:"display_name" json:"display_name,omitempty" yaml:"display_name"`
	Enabled              *bool               `bson:"enabled,omitempty" json:"enabled,omitempty" yaml:"enabled"`
	Private              *bool               `bson:"private,omitempty" json:"private,omitempty" yaml:"private"`
	Restricted           *bool               `bson:"restricted,omitempty" json:"restricted,omitempty" yaml:"restricted"`
	Owner                string              `bson:"owner_name" json:"owner_name" yaml:"owner"`
	Repo                 string              `bson:"repo_name" json:"repo_name" yaml:"repo"`
	Branch               string              `bson:"branch_name" json:"branch_name" yaml:"branch"`
	RemotePath           string              `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	PatchingDisabled     *bool               `bson:"patching_disabled,omitempty" json:"patching_disabled,omitempty"`
	RepotrackerDisabled  *bool               `bson:"repotracker_disabled,omitempty" json:"repotracker_disabled,omitempty" yaml:"repotracker_disabled"`
	DispatchingDisabled  *bool               `bson:"dispatching_disabled,omitempty" json:"dispatching_disabled,omitempty" yaml:"dispatching_disabled"`
	PRTestingEnabled     *bool               `bson:"pr_testing_enabled,omitempty" json:"pr_testing_enabled,omitempty" yaml:"pr_testing_enabled"`
	GithubChecksEnabled  *bool               `bson:"github_checks_enabled,omitempty" json:"github_checks_enabled,omitempty" yaml:"github_checks_enabled"`
	BatchTime            int                 `bson:"batch_time" json:"batch_time" yaml:"batchtime"`
	DeactivatePrevious   *bool               `bson:"deactivate_previous,omitempty" json:"deactivate_previous,omitempty" yaml:"deactivate_previous"`
	DefaultLogger        string              `bson:"default_logger" json:"default_logger" yaml:"default_logger"`
	NotifyOnBuildFailure *bool               `bson:"notify_on_failure,omitempty" json:"notify_on_failure,omitempty"`
	Triggers             []TriggerDefinition `bson:"triggers" json:"triggers"`
	// all aliases defined for the project
	PatchTriggerAliases []patch.PatchTriggerDefinition `bson:"patch_trigger_aliases" json:"patch_trigger_aliases"`
	// all PatchTriggerAliases applied to github patch intents
	GithubTriggerAliases    []string                  `bson:"github_trigger_aliases" json:"github_trigger_aliases"`
	PeriodicBuilds          []PeriodicBuildDefinition `bson:"periodic_builds" json:"periodic_builds"`
	CedarTestResultsEnabled *bool                     `bson:"cedar_test_results_enabled,omitempty" json:"cedar_test_results_enabled,omitempty" yaml:"cedar_test_results_enabled"`
	CommitQueue             CommitQueueParams         `bson:"commit_queue" json:"commit_queue" yaml:"commit_queue"`

	// Admins contain a list of users who are able to access the projects page.
	Admins []string `bson:"admins" json:"admins"`

	// SpawnHostScriptPath is a path to a script to optionally be run by users on hosts triggered from tasks.
	SpawnHostScriptPath string `bson:"spawn_host_script_path" json:"spawn_host_script_path" yaml:"spawn_host_script_path"`

	// TracksPushEvents, if true indicates that Repotracker is triggered by Github PushEvents for this project.
	// If a repo is enabled and this is what creates the hook, then TracksPushEvents will be set at the repo level.
	TracksPushEvents *bool `bson:"tracks_push_events" json:"tracks_push_events" yaml:"tracks_push_events"`

	// TaskSync holds settings for synchronizing task directories to S3.
	TaskSync TaskSyncOptions `bson:"task_sync" json:"task_sync" yaml:"task_sync"`

	// GitTagAuthorizedUsers contains a list of users who are able to create versions from git tags.
	GitTagAuthorizedUsers []string `bson:"git_tag_authorized_users" json:"git_tag_authorized_users"`
	GitTagAuthorizedTeams []string `bson:"git_tag_authorized_teams" json:"git_tag_authorized_teams"`
	GitTagVersionsEnabled *bool    `bson:"git_tag_versions_enabled,omitempty" json:"git_tag_versions_enabled,omitempty"`

	// RepoDetails contain the details of the status of the consistency
	// between what is in GitHub and what is in Evergreen
	RepotrackerError *RepositoryErrorDetails `bson:"repotracker_error" json:"repotracker_error"`

	// List of regular expressions describing files to ignore when caching historical test results
	FilesIgnoredFromCache []string `bson:"files_ignored_from_cache" json:"files_ignored_from_cache"`
	DisabledStatsCache    *bool    `bson:"disabled_stats_cache,omitempty" json:"disabled_stats_cache,omitempty"`

	// List of commands
	WorkstationConfig WorkstationConfig `bson:"workstation_config,omitempty" json:"workstation_config,omitempty"`

	// TaskAnnotationSettings holds settings for the file ticket button in the Task Annotations to call custom webhooks when clicked
	TaskAnnotationSettings evergreen.AnnotationsSettings `bson:"task_annotation_settings,omitempty" bson:"task_annotation_settings,omitempty"`

	// Plugin settings
	BuildBaronSettings evergreen.BuildBaronSettings `bson:"build_baron_settings,omitempty" json:"build_baron_settings,omitempty" yaml:"build_baron_settings,omitempty"`
	PerfEnabled        *bool                        `bson:"perf_enabled,omitempty" json:"perf_enabled,omitempty" yaml:"perf_enabled,omitempty"`

	// This is a temporary flag to enable individual projects to use repo settings
	UseRepoSettings bool   `bson:"use_repo_settings" json:"use_repo_settings" yaml:"use_repo_settings"`
	RepoRefId       string `bson:"repo_ref_id" json:"repo_ref_id" yaml:"repo_ref_id"`

	// The following fields are used by Evergreen and are not discoverable.
	// Hidden determines whether or not the project is discoverable/tracked in the UI
	Hidden *bool `bson:"hidden,omitempty" json:"hidden,omitempty"`
}

type CommitQueueParams struct {
	Enabled     *bool  `bson:"enabled" json:"enabled"`
	MergeMethod string `bson:"merge_method" json:"merge_method"`
	Message     string `bson:"message,omitempty" json:"message,omitempty"`
}

// TaskSyncOptions contains information about which features are allowed for
// syncing task directories to S3.
type TaskSyncOptions struct {
	ConfigEnabled *bool `bson:"config_enabled" json:"config_enabled"`
	PatchEnabled  *bool `bson:"patch_enabled" json:"patch_enabled"`
}

// RepositoryErrorDetails indicates whether or not there is an invalid revision and if there is one,
// what the guessed merge base revision is.
type RepositoryErrorDetails struct {
	Exists            bool   `bson:"exists" json:"exists"`
	InvalidRevision   string `bson:"invalid_revision" json:"invalid_revision"`
	MergeBaseRevision string `bson:"merge_base_revision" json:"merge_base_revision"`
}

type AlertConfig struct {
	Provider string `bson:"provider" json:"provider"` //e.g. e-mail, flowdock, SMS

	// Data contains provider-specific on how a notification should be delivered.
	// Typed as bson.M so that the appropriate provider can parse out necessary details
	Settings bson.M `bson:"settings" json:"settings"`
}

type TriggerDefinition struct {
	// completion of specified task(s) in the project listed here will cause a build in the current project
	Project string `bson:"project" json:"project"`
	Level   string `bson:"level" json:"level"` //build or task
	//used to enforce that only 1 version gets created from a given upstream commit + trigger combo
	DefinitionID string `bson:"definition_id" json:"definition_id"`

	// filters for this trigger
	BuildVariantRegex string `bson:"variant_regex,omitempty" json:"variant_regex,omitempty"`
	TaskRegex         string `bson:"task_regex,omitempty" json:"task_regex,omitempty"`
	Status            string `bson:"status,omitempty" json:"status,omitempty"`
	DateCutoff        *int   `bson:"date_cutoff,omitempty" json:"date_cutoff,omitempty"`

	// definitions for tasks to run for this trigger
	ConfigFile   string `bson:"config_file,omitempty" json:"config_file,omitempty"`
	Command      string `bson:"command,omitempty" json:"command,omitempty"`
	GenerateFile string `bson:"generate_file,omitempty" json:"generate_file,omitempty"`
	Alias        string `bson:"alias,omitempty" json:"alias,omitempty"`
}

type PeriodicBuildDefinition struct {
	ID            string    `bson:"id" json:"id"`
	ConfigFile    string    `bson:"config_file" json:"config_file"`
	IntervalHours int       `bson:"interval_hours" json:"interval_hours"`
	Alias         string    `bson:"alias,omitempty" json:"alias,omitempty"`
	Message       string    `bson:"message,omitempty" json:"message,omitempty"`
	NextRunTime   time.Time `bson:"next_run_time,omitempty" json:"next_run_time,omitempty"`
}

type WorkstationConfig struct {
	SetupCommands []WorkstationSetupCommand `bson:"setup_commands" json:"setup_commands"`
	GitClone      *bool                     `bson:"git_clone" json:"git_clone"`
}

type WorkstationSetupCommand struct {
	Command   string `bson:"command" json:"command"`
	Directory string `bson:"directory" json:"directory"`
}

func (a AlertConfig) GetSettingsMap() map[string]string {
	ret := make(map[string]string)
	for k, v := range a.Settings {
		ret[k] = fmt.Sprintf("%v", v)
	}
	return ret
}

type EmailAlertData struct {
	Recipients []string `bson:"recipients"`
}

var (
	// bson fields for the ProjectRef struct
	ProjectRefIdKey                      = bsonutil.MustHaveTag(ProjectRef{}, "Id")
	ProjectRefOwnerKey                   = bsonutil.MustHaveTag(ProjectRef{}, "Owner")
	ProjectRefRepoKey                    = bsonutil.MustHaveTag(ProjectRef{}, "Repo")
	ProjectRefBranchKey                  = bsonutil.MustHaveTag(ProjectRef{}, "Branch")
	ProjectRefEnabledKey                 = bsonutil.MustHaveTag(ProjectRef{}, "Enabled")
	ProjectRefPrivateKey                 = bsonutil.MustHaveTag(ProjectRef{}, "Private")
	ProjectRefRestrictedKey              = bsonutil.MustHaveTag(ProjectRef{}, "Restricted")
	ProjectRefBatchTimeKey               = bsonutil.MustHaveTag(ProjectRef{}, "BatchTime")
	ProjectRefIdentifierKey              = bsonutil.MustHaveTag(ProjectRef{}, "Identifier")
	ProjectRefRepoRefIdKey               = bsonutil.MustHaveTag(ProjectRef{}, "RepoRefId")
	ProjectRefDisplayNameKey             = bsonutil.MustHaveTag(ProjectRef{}, "DisplayName")
	ProjectRefDeactivatePreviousKey      = bsonutil.MustHaveTag(ProjectRef{}, "DeactivatePrevious")
	ProjectRefRemotePathKey              = bsonutil.MustHaveTag(ProjectRef{}, "RemotePath")
	ProjectRefHiddenKey                  = bsonutil.MustHaveTag(ProjectRef{}, "Hidden")
	ProjectRefRepotrackerError           = bsonutil.MustHaveTag(ProjectRef{}, "RepotrackerError")
	ProjectRefFilesIgnoredFromCacheKey   = bsonutil.MustHaveTag(ProjectRef{}, "FilesIgnoredFromCache")
	ProjectRefDisabledStatsCacheKey      = bsonutil.MustHaveTag(ProjectRef{}, "DisabledStatsCache")
	ProjectRefAdminsKey                  = bsonutil.MustHaveTag(ProjectRef{}, "Admins")
	ProjectRefGitTagAuthorizedUsersKey   = bsonutil.MustHaveTag(ProjectRef{}, "GitTagAuthorizedUsers")
	ProjectRefGitTagAuthorizedTeamsKey   = bsonutil.MustHaveTag(ProjectRef{}, "GitTagAuthorizedTeams")
	projectRefTracksPushEventsKey        = bsonutil.MustHaveTag(ProjectRef{}, "TracksPushEvents")
	projectRefDefaultLoggerKey           = bsonutil.MustHaveTag(ProjectRef{}, "DefaultLogger")
	projectRefCedarTestResultsEnabledKey = bsonutil.MustHaveTag(ProjectRef{}, "CedarTestResultsEnabled")
	projectRefPRTestingEnabledKey        = bsonutil.MustHaveTag(ProjectRef{}, "PRTestingEnabled")
	projectRefGithubChecksEnabledKey     = bsonutil.MustHaveTag(ProjectRef{}, "GithubChecksEnabled")
	projectRefGitTagVersionsEnabledKey   = bsonutil.MustHaveTag(ProjectRef{}, "GitTagVersionsEnabled")
	projectRefUseRepoSettingsKey         = bsonutil.MustHaveTag(ProjectRef{}, "UseRepoSettings")
	projectRefRepotrackerDisabledKey     = bsonutil.MustHaveTag(ProjectRef{}, "RepotrackerDisabled")
	projectRefCommitQueueKey             = bsonutil.MustHaveTag(ProjectRef{}, "CommitQueue")
	projectRefTaskSyncKey                = bsonutil.MustHaveTag(ProjectRef{}, "TaskSync")
	projectRefPatchingDisabledKey        = bsonutil.MustHaveTag(ProjectRef{}, "PatchingDisabled")
	projectRefDispatchingDisabledKey     = bsonutil.MustHaveTag(ProjectRef{}, "DispatchingDisabled")
	projectRefNotifyOnFailureKey         = bsonutil.MustHaveTag(ProjectRef{}, "NotifyOnBuildFailure")
	projectRefSpawnHostScriptPathKey     = bsonutil.MustHaveTag(ProjectRef{}, "SpawnHostScriptPath")
	projectRefTriggersKey                = bsonutil.MustHaveTag(ProjectRef{}, "Triggers")
	projectRefPatchTriggerAliasesKey     = bsonutil.MustHaveTag(ProjectRef{}, "PatchTriggerAliases")
	projectRefGithubTriggerAliasesKey    = bsonutil.MustHaveTag(ProjectRef{}, "GithubTriggerAliases")
	projectRefPeriodicBuildsKey          = bsonutil.MustHaveTag(ProjectRef{}, "PeriodicBuilds")
	projectRefWorkstationConfigKey       = bsonutil.MustHaveTag(ProjectRef{}, "WorkstationConfig")
	projectRefTaskAnnotationSettingsKey  = bsonutil.MustHaveTag(ProjectRef{}, "TaskAnnotationSettings")
	projectRefBuildBaronSettingsKey      = bsonutil.MustHaveTag(ProjectRef{}, "BuildBaronSettings")
	projectRefPerfEnabledKey             = bsonutil.MustHaveTag(ProjectRef{}, "PerfEnabled")

	commitQueueEnabledKey       = bsonutil.MustHaveTag(CommitQueueParams{}, "Enabled")
	triggerDefinitionProjectKey = bsonutil.MustHaveTag(TriggerDefinition{}, "Project")
)

func (p *ProjectRef) IsEnabled() bool {
	return utility.FromBoolPtr(p.Enabled)
}

func (p *ProjectRef) IsPrivate() bool {
	return utility.FromBoolPtr(p.Private)
}

func (p *ProjectRef) IsRestricted() bool {
	return utility.FromBoolPtr(p.Restricted)
}

func (p *ProjectRef) IsPatchingDisabled() bool {
	return utility.FromBoolPtr(p.PatchingDisabled)
}

func (p *ProjectRef) IsRepotrackerDisabled() bool {
	return utility.FromBoolPtr(p.RepotrackerDisabled)
}

func (p *ProjectRef) IsDispatchingDisabled() bool {
	return utility.FromBoolPtr(p.DispatchingDisabled)
}

func (p *ProjectRef) IsPRTestingEnabled() bool {
	return utility.FromBoolPtr(p.PRTestingEnabled)
}

func (p *ProjectRef) IsGithubChecksEnabled() bool {
	return utility.FromBoolPtr(p.GithubChecksEnabled)
}

func (p *ProjectRef) ShouldDeactivatePrevious() bool {
	return utility.FromBoolPtr(p.DeactivatePrevious)
}

func (p *ProjectRef) ShouldNotifyOnBuildFailure() bool {
	return utility.FromBoolPtr(p.NotifyOnBuildFailure)
}

func (p *ProjectRef) IsCedarTestResultsEnabled() bool {
	return utility.FromBoolPtr(p.CedarTestResultsEnabled)
}

func (p *ProjectRef) IsGitTagVersionsEnabled() bool {
	return utility.FromBoolPtr(p.GitTagVersionsEnabled)
}

func (p *ProjectRef) IsStatsCacheDisabled() bool {
	return utility.FromBoolPtr(p.DisabledStatsCache)
}

func (p *ProjectRef) IsHidden() bool {
	return utility.FromBoolPtr(p.Hidden)
}

func (p *ProjectRef) DoesTrackPushEvents() bool {
	return utility.FromBoolPtr(p.TracksPushEvents)
}

func (p *ProjectRef) IsPerfEnabled() bool {
	return utility.FromBoolPtr(p.PerfEnabled)
}

func (p *CommitQueueParams) IsEnabled() bool {
	return utility.FromBoolPtr(p.Enabled)
}

func (ts *TaskSyncOptions) IsPatchEnabled() bool {
	return utility.FromBoolPtr(ts.PatchEnabled)
}

func (ts *TaskSyncOptions) IsConfigEnabled() bool {
	return utility.FromBoolPtr(ts.ConfigEnabled)
}

func (c *WorkstationConfig) ShouldGitClone() bool {
	return utility.FromBoolPtr(c.GitClone)
}

func (p *ProjectRef) AliasesNeeded() bool {
	return p.IsGithubChecksEnabled() || p.IsGitTagVersionsEnabled() || p.IsGithubChecksEnabled() || p.IsPRTestingEnabled()
}

const (
	ProjectRefCollection     = "project_ref"
	ProjectTriggerLevelTask  = "task"
	ProjectTriggerLevelBuild = "build"
	intervalPrefix           = "@every"
	maxBatchTime             = 153722867 // math.MaxInt64 / 60 / 1_000_000_000
)

type ProjectPageSection string

const (
	ProjectPageGeneralSection        = "general"
	ProjectPageAccessSection         = "access"
	ProjectPageVariablesSection      = "variables"
	ProjectPageGithubAndCQSection    = "github_and_commit_queue"
	ProjectPageNotificationsSection  = "notifications"
	ProjectPagePatchAliasSection     = "patch_alias"
	ProjectPageWorkstationsSection   = "workstations"
	ProjectPageTriggersSection       = "triggers"
	ProjectPagePeriodicBuildsSection = "periodic-builds"
)

var adminPermissions = gimlet.Permissions{
	evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
	evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
	evergreen.PermissionPatches:         evergreen.PatchSubmit.Value,
	evergreen.PermissionLogs:            evergreen.LogsView.Value,
}

func (projectRef *ProjectRef) Insert() error {
	return db.Insert(ProjectRefCollection, projectRef)
}

func (p *ProjectRef) Add(creator *user.DBUser) error {
	if p.Id == "" {
		p.Id = mgobson.NewObjectId().Hex()
	}

	// if a hidden project exists for this configuration, use that ID
	if p.Owner != "" && p.Repo != "" && p.Branch != "" {
		hidden, err := FindHiddenProjectRefByOwnerRepoAndBranch(p.Owner, p.Repo, p.Branch)
		if err != nil {
			return errors.Wrapf(err, "error querying for hidden project")
		}
		if hidden != nil {
			p.Id = hidden.Id
			err := p.Upsert()
			if err != nil {
				return errors.Wrapf(err, "error upserting project ref '%s'", hidden.Id)
			}
			if creator != nil {
				return p.UpdateAdminRoles([]string{creator.Id}, nil)
			}
			return nil
		}
	}

	err := db.Insert(ProjectRefCollection, p)
	if err != nil {
		return errors.Wrap(err, "Error inserting project ref")
	}
	return p.AddPermissions(creator)
}

func (p *ProjectRef) GetPatchTriggerAlias(aliasName string) (patch.PatchTriggerDefinition, bool) {
	for _, alias := range p.PatchTriggerAliases {
		if alias.Alias == aliasName {
			return alias, true
		}
	}

	return patch.PatchTriggerDefinition{}, false
}

// MergeWithParserProject looks up the parser project with the given project ref id and modifies
// the project ref scanning for any properties that can be set on both project ref and project parser.
// Any values that are set at the project parser level will be set on the project ref.
func (p *ProjectRef) MergeWithParserProject(version string) error {
	lookupVersion := false
	if version == "" {
		lastGoodVersion, err := FindVersionByLastKnownGoodConfig(p.Id, -1)
		if err != nil || lastGoodVersion == nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    fmt.Sprintf("Unable to retrieve last good version for project '%s'", p.Id),
				"project_id": p.Id,
				"version":    version,
			}))
			return err
		}
		version = lastGoodVersion.Id
		lookupVersion = true
	}
	parserProject, err := ParserProjectFindOneById(version)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":        fmt.Sprintf("Error retrieving parser project for version '%s'", version),
			"project_id":     p.Id,
			"version":        version,
			"lookup_version": lookupVersion,
		}))
		return err
	}
	if parserProject != nil {
		if parserProject.PerfEnabled != nil {
			p.PerfEnabled = parserProject.PerfEnabled
		}
		if parserProject.DeactivatePrevious != nil {
			p.DeactivatePrevious = parserProject.DeactivatePrevious
		}
		if parserProject.TaskAnnotationSettings != nil {
			p.TaskAnnotationSettings = *parserProject.TaskAnnotationSettings
		}
	}
	return nil
}

// AttachToRepo adds the branch to the relevant repo scopes, and updates the project to point to the repo.
// Any values that previously were unset will now use the repo value.
// If no repo ref currently exists, the user attaching it will be added as the repo ref admin.
func (p *ProjectRef) AttachToRepo(u *user.DBUser) error {
	before, err := GetProjectSettingsById(p.Id, false)
	if err != nil {
		return errors.Wrap(err, "error getting before project settings event")
	}
	if err := p.AddToRepoScope(u); err != nil {
		return err
	}
	err = db.UpdateId(ProjectRefCollection, p.Id, bson.M{
		"$set": bson.M{
			projectRefUseRepoSettingsKey: true,
			ProjectRefRepoRefIdKey:       p.RepoRefId, // this is set locally in AddToRepoScope
		},
	})
	if err != nil {
		return errors.Wrap(err, "error attaching repo to scope")
	}
	p.UseRepoSettings = true
	return GetAndLogProjectModified(p.Id, u.Id, false, before)
}

// AddToRepoScope adds the branch to the unrestricted branches under repo scope, adds repo view permission for
// branch admins, and adds branch edit access for repo admins.
func (p *ProjectRef) AddToRepoScope(u *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	repoRef, err := FindRepoRefByOwnerAndRepo(p.Owner, p.Repo)
	if err != nil {
		return errors.Wrapf(err, "error finding repo ref '%s'", p.RepoRefId)
	}
	if repoRef == nil {
		repoRef, err = p.createNewRepoRef(u)
		if err != nil {
			return errors.Wrapf(err, "error creating new repo ref")
		}
	}
	if p.RepoRefId == "" {
		p.RepoRefId = repoRef.Id
	}

	// add the project to the repo admin scope
	if err := rm.AddResourceToScope(GetRepoAdminScope(p.RepoRefId), p.Id); err != nil {
		return errors.Wrapf(err, "error adding resource to repo '%s' admin scope", p.RepoRefId)
	}
	// only give branch admins view access if the repo isn't restricted
	if !repoRef.IsRestricted() {
		if err := addViewRepoPermissionsToBranchAdmins(p.RepoRefId, p.Admins); err != nil {
			return errors.Wrapf(err, "error giving branch '%s' admins view permission for repo '%s'", p.Id, p.RepoRefId)
		}
	}
	// if the branch is unrestricted, add it to this scope so users who requested all-repo permissions have access
	if !p.IsRestricted() {
		if err := rm.AddResourceToScope(GetUnrestrictedBranchProjectsScope(p.RepoRefId), p.Id); err != nil {
			return errors.Wrap(err, "error adding resource to unrestricted branches scope")
		}
	}
	return nil
}

// DetachFromRepo removes the branch from the relevant repo scopes, and updates the project to not point to the repo.
// Any values that previously defaulted to repo will have the repo value explicitly set.
func (p *ProjectRef) DetachFromRepo(u *user.DBUser) error {
	before, err := GetProjectSettingsById(p.Id, false)
	if err != nil {
		return errors.Wrap(err, "error getting before project settings event")
	}

	// remove from relevant repo scopes
	if err = p.RemoveFromRepoScope(); err != nil {
		return err
	}
	p.UseRepoSettings = false
	p.RepoRefId = ""

	mergedProject, err := FindMergedProjectRef(p.Id)
	if err != nil {
		return errors.Wrap(err, "error finding merged project ref")
	}
	if mergedProject == nil {
		return errors.Errorf("project ref '%s' doesn't exist", p.Id)
	}

	// Save repo variables that don't exist in the repo as the project variables.
	// Wait to save merged project until we've gotten the variables.
	mergedVars, err := FindMergedProjectVars(before.ProjectRef.Id)
	if err != nil {
		return errors.Wrap(err, "error finding merged project vars")
	}

	mergedProject.UseRepoSettings = false
	mergedProject.RepoRefId = ""
	if err = mergedProject.Upsert(); err != nil {
		return errors.Wrap(err, "error detaching project from repo")
	}

	// catch any resulting errors so that we log before returning
	catcher := grip.NewBasicCatcher()
	if mergedVars != nil {
		_, err = mergedVars.Upsert()
		catcher.Wrap(err, "error saving merged vars")
	}

	if len(before.Subscriptions) == 0 {
		// Save repo subscriptions as project subscriptions if none exist
		subs, err := event.FindSubscriptionsByOwner(before.ProjectRef.RepoRefId, event.OwnerTypeProject)
		catcher.Wrap(err, "error finding repo subscriptions")

		for _, s := range subs {
			s.ID = ""
			s.Owner = p.Id
			catcher.Add(s.Upsert())
		}
	}

	// Handle each category of aliases as it's own case
	repoAliases, err := FindAliasesForProject(before.ProjectRef.RepoRefId)
	catcher.Wrap(err, "error finding repo aliases")

	hasInternalAliases := map[string]bool{}
	hasPatchAlias := false
	for _, a := range before.Aliases {
		if utility.StringSliceContains(evergreen.InternalAliases, a.Alias) {
			hasInternalAliases[a.Alias] = true
		} else { // if it's not an internal alias, it's a patch alias. Only add repo patch aliases if no patch aliases exist for the project.
			hasPatchAlias = true
		}
	}
	repoAliasesToCopy := []ProjectAlias{}
	for _, internalAlias := range evergreen.InternalAliases {
		// if the branch doesn't have the internal alias set, add any that exist for the repo
		if !hasInternalAliases[internalAlias] {
			for _, repoAlias := range repoAliases {
				if repoAlias.Alias == internalAlias {
					repoAliasesToCopy = append(repoAliasesToCopy, repoAlias)
				}
			}
		}
	}
	if !hasPatchAlias {
		// if the branch doesn't have patch aliases set, add any non-internal aliases that exist for the repo
		for _, repoAlias := range repoAliases {
			if !utility.StringSliceContains(evergreen.InternalAliases, repoAlias.Alias) {
				repoAliasesToCopy = append(repoAliasesToCopy, repoAlias)
			}
		}
	}
	catcher.Add(UpsertAliasesForProject(repoAliasesToCopy, p.Id))

	catcher.Add(GetAndLogProjectModified(p.Id, u.Id, false, before))
	return catcher.Resolve()
}

func (p *ProjectRef) ChangeOwnerRepo(u *user.DBUser) error {
	before, err := GetProjectSettingsById(p.Id, false)
	if err != nil {
		return errors.Wrap(err, "error getting before project settings event")
	}

	allowedOrgs := evergreen.GetEnvironment().Settings().GithubOrgs
	if err := p.ValidateOwnerAndRepo(allowedOrgs); err != nil {

	}
	if p.UseRepoSettings {
		if err := p.RemoveFromRepoScope(); err != nil {
			return errors.Wrapf(err, "error removing project from old repo scope")
		}
		p.RepoRefId = "" // will reassign this in add
		if err := p.AddToRepoScope(u); err != nil {
			return errors.Wrapf(err, "error addding project to new repo scope")
		}
	}
	update := bson.M{
		"$set": bson.M{
			ProjectRefOwnerKey:     p.Owner,
			ProjectRefRepoKey:      p.Repo,
			ProjectRefRepoRefIdKey: p.RepoRefId,
		},
	}
	if err := db.UpdateId(ProjectRefCollection, p.Id, update); err != nil {
		return errors.Wrap(err, "error updating owner/repo in the DB")
	}
	return GetAndLogProjectModified(p.Id, u.Id, false, before)
}

// RemoveFromRepoScope removes the branch from the unrestricted branches under repo scope, removes repo view permission
// for branch admins, and removes branch edit access for repo admins.
func (p *ProjectRef) RemoveFromRepoScope() error {
	if p.RepoRefId == "" {
		return nil
	}
	rm := evergreen.GetEnvironment().RoleManager()
	if !p.IsRestricted() {
		if err := rm.RemoveResourceFromScope(GetUnrestrictedBranchProjectsScope(p.RepoRefId), p.Id); err != nil {
			return errors.Wrap(err, "error removing resource from unrestricted branches scope")
		}
	}
	if err := removeViewRepoPermissionsFromBranchAdmins(p.RepoRefId, p.Admins); err != nil {
		return errors.Wrap(err, "error removing view repo permissions from branch admins")
	}
	return errors.Wrapf(rm.RemoveResourceFromScope(GetRepoAdminScope(p.RepoRefId), p.Id),
		"error removing from repo '%s' admin scope", p.Repo)
}

func (p *ProjectRef) AddPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	parentScope := evergreen.UnrestrictedProjectsScope
	if p.IsRestricted() {
		parentScope = evergreen.RestrictedProjectsScope
	}
	if err := rm.AddResourceToScope(parentScope, p.Id); err != nil {
		return errors.Wrapf(err, "unable to add '%s' to the '%s' scope", p.Id, parentScope)
	}

	// add scope for the branch-level project configurations
	newScope := gimlet.Scope{
		ID:        fmt.Sprintf("project_%s", p.Id),
		Resources: []string{p.Id},
		Name:      p.Id,
		Type:      evergreen.ProjectResourceType,
	}
	if err := rm.AddScope(newScope); err != nil {
		return errors.Wrapf(err, "error adding scope for project '%s'", p.Id)
	}
	newRole := gimlet.Role{
		ID:          fmt.Sprintf("admin_project_%s", p.Id),
		Scope:       newScope.ID,
		Permissions: adminPermissions,
	}
	if creator != nil {
		newRole.Owners = []string{creator.Id}
	}
	if err := rm.UpdateRole(newRole); err != nil {
		return errors.Wrapf(err, "error adding admin role for project '%s'", p.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newRole.ID); err != nil {
			return errors.Wrapf(err, "error adding role '%s' to user '%s'", newRole.ID, creator.Id)
		}
	}
	if p.UseRepoSettings {
		if err := p.AddToRepoScope(creator); err != nil {
			return errors.Wrapf(err, "error adding project to repo '%s'", p.RepoRefId)
		}
	}
	return nil
}

func (projectRef *ProjectRef) Update() error {
	return db.Update(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdKey: projectRef.Id,
		},
		projectRef,
	)
}

func (projectRef *ProjectRef) checkDefaultLogger() {
	if projectRef.DefaultLogger == "" {
		projectRef.DefaultLogger = evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger
	}
}

func findOneProjectRefQ(query db.Q) (*ProjectRef, error) {
	projectRef := &ProjectRef{}
	err := db.FindOneQ(ProjectRefCollection, query, projectRef)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	projectRef.checkDefaultLogger()
	return projectRef, err

}

// FindBranchProjectRef gets a project ref given the project identifier.
// This returns only branch-level settings; to include repo settings, use FindMergedProjectRef.
func FindBranchProjectRef(identifier string) (*ProjectRef, error) {
	return findOneProjectRefQ(byId(identifier))
}

// FindMergedProjectRef also finds the repo ref settings and merges relevant fields.
func FindMergedProjectRef(identifier string) (*ProjectRef, error) {
	pRef, err := FindBranchProjectRef(identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding project ref '%s'", identifier)
	}
	if pRef == nil {
		return nil, nil
	}
	if pRef.UseRepoSettings {
		repoRef, err := FindOneRepoRef(pRef.RepoRefId)
		if err != nil {
			return nil, errors.Wrapf(err, "error finding repo ref '%s' for project '%s'", pRef.RepoRefId, pRef.Identifier)
		}
		if repoRef == nil {
			return nil, errors.Errorf("repo ref '%s' does not exist for project '%s'", pRef.RepoRefId, pRef.Identifier)
		}
		return mergeBranchAndRepoSettings(pRef, repoRef)
	}
	return pRef, nil
}

// GetProjectRefMergedWithRepo merges the project with the repo that matches it, if one exists.
// Otherwise, it will return the project as given.
func GetProjectRefMergedWithRepo(pRef ProjectRef) (*ProjectRef, error) {
	if !pRef.UseRepoSettings {
		return &pRef, nil
	}
	if pRef.RepoRefId != "" {
		repoRef, err := FindOneRepoRef(pRef.RepoRefId)
		if err != nil {
			return nil, errors.Wrapf(err, "error finding repo ref '%s'", pRef.RepoRefId)
		}
		if repoRef == nil {
			return nil, errors.Errorf("repo ref '%s' does not exist", pRef.RepoRefId)
		}
		return mergeBranchAndRepoSettings(&pRef, repoRef)
	}
	repoRef, err := FindRepoRefByOwnerAndRepo(pRef.Owner, pRef.Repo)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding repo ref for repo '%s/%s'", pRef.Owner, pRef.Repo)
	}
	if repoRef == nil {
		return &pRef, nil
	}
	pRef.RepoRefId = repoRef.Id
	return mergeBranchAndRepoSettings(&pRef, repoRef)
}

// If the setting is not defined in the project, default to the repo settings.
func mergeBranchAndRepoSettings(pRef *ProjectRef, repoRef *RepoRef) (*ProjectRef, error) {
	var err error
	defer func() {
		err = recovery.HandlePanicWithError(recover(), err, "project and repo structures do not match")
	}()
	reflectedBranch := reflect.ValueOf(pRef).Elem()
	reflectedRepo := reflect.ValueOf(repoRef).Elem().Field(0) // specifically references the ProjectRef part of RepoRef

	recursivelySetUndefinedFields(reflectedBranch, reflectedRepo)
	return pRef, err
}

func recursivelySetUndefinedFields(structToSet, structToDefaultFrom reflect.Value) {
	// Iterate through each field of the struct.
	for i := 0; i < structToSet.NumField(); i++ {
		branchField := structToSet.Field(i)

		// If the field isn't set, use the default field.
		// Note for pointers and maps, we consider the field undefined if the item is nil or empty length,
		// and we don't check for subfields. This allows us to group some settings together as defined or undefined.
		if util.IsFieldUndefined(branchField) {
			reflectedField := structToDefaultFrom.Field(i)
			branchField.Set(reflectedField)

			// If the field is a struct and isn't undefined, then we check each subfield recursively.
		} else if branchField.Kind() == reflect.Struct {
			recursivelySetUndefinedFields(branchField, structToDefaultFrom.Field(i))
		}
	}
}

func setRepoFieldsFromProjects(repoRef *RepoRef, projectRefs []ProjectRef) {
	if len(projectRefs) == 0 {
		return
	}
	reflectedRepo := reflect.ValueOf(repoRef).Elem().Field(0) // specifically references the ProjectRef part of RepoRef
	for i := 0; i < reflectedRepo.NumField(); i++ {
		// for each field in the repo, look at each field in the project ref
		var firstVal reflect.Value
		allEqual := true
		for j, pRef := range projectRefs {
			reflectedBranchField := reflect.ValueOf(pRef).Field(i)
			if j == 0 {
				firstVal = reflectedBranchField
			} else if !reflect.DeepEqual(firstVal.Interface(), reflectedBranchField.Interface()) {
				allEqual = false
				break
			}
		}
		// if we got to the end of the loop, then all values are the same, so we can assign it to reflectedRepo
		if allEqual {
			reflectedRepo.Field(i).Set(firstVal)
		}
	}
}

func (p *ProjectRef) createNewRepoRef(u *user.DBUser) (repoRef *RepoRef, err error) {
	repoRef = &RepoRef{ProjectRef{
		Enabled: utility.TruePtr(),
		Admins:  []string{},
	}}

	allEnabledProjects, err := FindMergedEnabledProjectRefsByOwnerAndRepo(p.Owner, p.Repo)
	if err != nil {
		return nil, errors.Wrap(err, "error finding all enabled projects")
	}
	// for every setting in the project ref, if all enabled projects have the same setting, then use that
	defer func() {
		err = recovery.HandlePanicWithError(recover(), err, "project and repo structures do not match")
	}()
	setRepoFieldsFromProjects(repoRef, allEnabledProjects)
	if !utility.StringSliceContains(repoRef.Admins, u.Username()) {
		repoRef.Admins = append(repoRef.Admins, u.Username())
	}
	// some fields shouldn't be set from projects
	repoRef.Id = mgobson.NewObjectId().Hex()
	repoRef.UseRepoSettings = false
	// set explicitly in case no project is enabled
	repoRef.Owner = p.Owner
	repoRef.Repo = p.Repo

	// creates scope and give user admin access to repo
	if err = repoRef.Add(u); err != nil {
		return nil, errors.Wrapf(err, "problem adding new repo repo ref for '%s/%s'", p.Owner, p.Repo)
	}

	enabledProjectIds := []string{}
	for _, p := range allEnabledProjects {
		enabledProjectIds = append(enabledProjectIds, p.Id)
	}
	commonProjectVars, err := getCommonProjectVariables(enabledProjectIds)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting common project variables")
	}
	commonProjectVars.Id = repoRef.Id
	if err = commonProjectVars.Insert(); err != nil {
		return nil, errors.Wrap(err, "error inserting project variables for repo")
	}

	commonAliases, err := getCommonAliases(enabledProjectIds)
	if err != nil {
		return nil, errors.Wrap(err, "error getting common project aliases")
	}
	for _, a := range commonAliases {
		a.ProjectID = repoRef.Id
		if err = a.Upsert(); err != nil {
			return nil, errors.Wrapf(err, "error upserting alias for repo")
		}
	}

	return repoRef, nil
}

func getCommonAliases(projectIds []string) (ProjectAliases, error) {
	commonAliases := []ProjectAlias{}
	for i, id := range projectIds {
		aliases, err := FindAliasesForProject(id)
		if err != nil {
			return nil, errors.Wrap(err, "error finding aliases for project")
		}
		if i == 0 {
			commonAliases = aliases
			continue
		}
		for j := len(commonAliases) - 1; j >= 0; j-- {
			// look to see if this alias exists in the each project and if not remove it
			if !aliasSliceContains(aliases, commonAliases[j]) {
				commonAliases = append(commonAliases[:j], commonAliases[j+1:]...)
			}
		}
		if len(commonAliases) == 0 {
			return nil, nil
		}
	}

	return commonAliases, nil
}

func aliasSliceContains(slice []ProjectAlias, item ProjectAlias) bool {
	for _, each := range slice {
		if each.RemotePath != item.RemotePath || each.Alias != item.Alias || each.GitTag != item.GitTag ||
			each.Variant != item.Variant || each.Task != item.Task {
			continue
		}

		if len(each.VariantTags) != len(item.VariantTags) || len(each.TaskTags) != len(item.TaskTags) {
			continue
		}
		if len(each.VariantTags) != len(utility.StringSliceIntersection(each.VariantTags, item.VariantTags)) {
			continue
		}
		if len(each.TaskTags) != len(utility.StringSliceIntersection(each.TaskTags, item.TaskTags)) {
			continue
		}
		return true
	}
	return false
}

func getCommonProjectVariables(projectIds []string) (*ProjectVars, error) {
	// add in project variables and aliases here
	commonProjectVariables := map[string]string{}
	commonPrivate := map[string]bool{}
	commonRestricted := map[string]bool{}
	for i, id := range projectIds {
		vars, err := FindOneProjectVars(id)
		if err != nil {
			return nil, errors.Wrapf(err, "error finding variables for project '%s'", id)
		}
		if vars == nil {
			continue
		}
		if i == 0 {
			if vars.Vars != nil {
				commonProjectVariables = vars.Vars
			}
			if vars.PrivateVars != nil {
				commonPrivate = vars.PrivateVars
			}
			if vars.RestrictedVars != nil {
				commonRestricted = vars.RestrictedVars
			}
			continue
		}
		for key, val := range commonProjectVariables {
			// if the key is private/restricted in any of the projects, make it private/restricted in the repo
			if vars.Vars[key] == val {
				if vars.PrivateVars[key] {
					commonPrivate[key] = true
				}
				if vars.RestrictedVars[key] {
					commonRestricted[key] = true
				}
			} else {
				// remove any variables from the common set that aren't in all the project refs
				delete(commonProjectVariables, key)
			}
		}
	}
	return &ProjectVars{
		Vars:           commonProjectVariables,
		PrivateVars:    commonPrivate,
		RestrictedVars: commonRestricted,
	}, nil
}

func GetIdForProject(identifier string) (string, error) {
	pRef, err := findOneProjectRefQ(byId(identifier).WithFields(ProjectRefIdKey))
	if err != nil {
		return "", err
	}
	if pRef == nil {
		return "", errors.Errorf("project '%s' does not exist", identifier)
	}
	return pRef.Id, nil
}

func GetIdentifierForProject(id string) (string, error) {
	pRef, err := findOneProjectRefQ(byId(id).WithFields(ProjectRefIdentifierKey))
	if err != nil {
		return "", err
	}
	if pRef == nil {
		return "", errors.Errorf("project '%s' does not exist", id)
	}
	return pRef.Identifier, nil
}

func CountProjectRefsWithIdentifier(identifier string) (int, error) {
	return db.CountQ(ProjectRefCollection, byId(identifier))
}

func FindFirstProjectRef() (*ProjectRef, error) {
	projectRef := &ProjectRef{}
	pipeline := projectRefPipelineForValueIsBool(ProjectRefPrivateKey, RepoRefPrivateKey, false)
	pipeline = append(pipeline, bson.M{"$sort": "-" + ProjectRefDisplayNameKey}, bson.M{"$limit": 1})
	err := db.Aggregate(
		ProjectRefCollection,
		pipeline,
		projectRef,
	)

	if err != nil {
		return nil, errors.Wrapf(err, "error aggregating project ref")
	}
	projectRef.checkDefaultLogger()

	return projectRef, nil
}

// FindAllMergedTrackedProjectRefs returns all project refs in the db
// that are currently being tracked (i.e. their project files
// still exist and the project is not hidden).
// Can't hide a repo without hiding the branches, so don't need to aggregate here.
func FindAllMergedTrackedProjectRefs() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	q := db.Query(bson.M{ProjectRefHiddenKey: bson.M{"$ne": true}})
	err := db.FindAllQ(ProjectRefCollection, q, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

func addLoggerAndRepoSettingsToProjects(pRefs []ProjectRef) ([]ProjectRef, error) {
	repoRefs := map[string]*RepoRef{} // cache repoRefs by id
	for i, pRef := range pRefs {
		pRefs[i].checkDefaultLogger()
		if pRefs[i].UseRepoSettings {
			repoRef := repoRefs[pRef.RepoRefId]
			if repoRef == nil {
				var err error
				repoRef, err = FindOneRepoRef(pRef.RepoRefId)
				if err != nil {
					return nil, errors.Wrapf(err, "error finding repo ref '%s' for project '%s'", pRef.RepoRefId, pRef.Identifier)
				}
				if repoRef == nil {
					return nil, errors.Errorf("repo ref '%s' does not exist for project '%s'", pRef.RepoRefId, pRef.Identifier)
				}
				repoRefs[pRef.RepoRefId] = repoRef
			}
			mergedProject, err := mergeBranchAndRepoSettings(&pRefs[i], repoRef)
			if err != nil {
				return nil, errors.Wrapf(err, "error merging settings")
			}
			pRefs[i] = *mergedProject
		}
	}
	return pRefs, nil
}

// FindAllMergedProjectRefs returns all project refs in the db, with repo ref information merged
func FindAllMergedProjectRefs() ([]ProjectRef, error) {
	return FindProjectRefsQ(bson.M{})
}

func FindProjectRefsByIds(ids []string) ([]ProjectRef, error) {
	return FindProjectRefsQ(bson.M{
		ProjectRefIdKey: bson.M{
			"$in": ids,
		},
	})
}

func FindProjectRefsQ(filter bson.M) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	q := db.Query(filter)
	err := db.FindAllQ(ProjectRefCollection, q, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

func byOwnerAndRepo(owner, repoName string) bson.M {
	return bson.M{
		ProjectRefOwnerKey: owner,
		ProjectRefRepoKey:  repoName,
	}
}
func byOwnerRepoAndBranch(owner, repoName, branch string) bson.M {
	return bson.M{
		ProjectRefOwnerKey:  owner,
		ProjectRefRepoKey:   repoName,
		ProjectRefBranchKey: branch,
	}
}

func byId(identifier string) db.Q {
	return db.Query(bson.M{
		"$or": []bson.M{
			{ProjectRefIdKey: identifier},
			{ProjectRefIdentifierKey: identifier},
		},
	})
}

// FindMergedEnabledProjectRefsByRepoAndBranch finds ProjectRefs with matching repo/branch
// that are enabled, and merges repo information.
func FindMergedEnabledProjectRefsByRepoAndBranch(owner, repoName, branch string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	pipeline := []bson.M{{"$match": byOwnerRepoAndBranch(owner, repoName, branch)}}
	pipeline = append(pipeline, projectRefPipelineForValueIsBool(ProjectRefEnabledKey, RepoRefEnabledKey, true)...)
	err := db.Aggregate(ProjectRefCollection, pipeline, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

// FindMergedProjectRefsThatUseRepoSettingsByRepoAndBranch finds ProjectRef with matching repo/branch that
// rely on the repo configuration, and merges that info.
func FindMergedProjectRefsThatUseRepoSettingsByRepoAndBranch(owner, repoName, branch string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	q := byOwnerRepoAndBranch(owner, repoName, branch)
	q[projectRefUseRepoSettingsKey] = true
	pipeline := []bson.M{{"$match": q}}
	err := db.Aggregate(ProjectRefCollection, pipeline, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

func FindBranchAdminsForRepo(repoId string) ([]string, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAllQ(
		ProjectRefCollection,
		db.Query(bson.M{
			ProjectRefRepoRefIdKey:       repoId,
			projectRefUseRepoSettingsKey: true,
		}).WithFields(ProjectRefAdminsKey),
		&projectRefs,
	)
	if err != nil {
		return nil, err
	}
	allBranchAdmins := []string{}
	for _, pRef := range projectRefs {
		allBranchAdmins = append(allBranchAdmins, pRef.Admins...)
	}
	return utility.UniqueStrings(allBranchAdmins), nil
}

// Find repos that have that trigger / are enabled
// find projects that have this repo ID and nil triggers,OR that have the trigger
func FindDownstreamProjects(project string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.Aggregate(ProjectRefCollection, projectRefPipelineForMatchingTrigger(project), &projectRefs)
	if err != nil {
		return nil, err
	}

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}
	return projectRefs, err
}

// FindOneProjectRefByRepoAndBranchWithPRTesting finds a single ProjectRef with matching
// repo/branch that is enabled and setup for PR testing.
func FindOneProjectRefByRepoAndBranchWithPRTesting(owner, repo, branch string) (*ProjectRef, error) {
	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}
	for _, p := range projectRefs {
		if p.IsPRTestingEnabled() {
			p.checkDefaultLogger()
			return &p, nil
		}
	}
	if len(projectRefs) > 0 {
		grip.Debug(message.Fields{
			"source":  "find project ref for PR testing",
			"message": "project ref enabled but pr testing not enabled",
			"owner":   owner,
			"repo":    repo,
			"branch":  branch,
		})
		return nil, nil
	}

	// if no projects are enabled, check if the repo has PR testing enabled, in which case we can use a disabled/hidden project.
	repoRef, err := FindRepoRefByOwnerAndRepo(owner, repo)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding merged repo refs for repo '%s/%s'", owner, repo)
	}
	if repoRef == nil || !repoRef.IsEnabled() || !repoRef.IsPRTestingEnabled() || repoRef.RemotePath == "" {
		grip.Debug(message.Fields{
			"source":  "find project ref for PR testing",
			"message": "repo ref not configured for PR testing untracked branches",
			"owner":   owner,
			"repo":    repo,
			"branch":  branch,
		})
		return nil, nil
	}

	projectRefs, err = FindMergedProjectRefsThatUseRepoSettingsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding merged all project refs for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}

	// if a disabled project exists, then return early
	var hiddenProject *ProjectRef
	for i, p := range projectRefs {
		if !p.IsEnabled() && !p.IsHidden() {
			grip.Debug(message.Fields{
				"source":  "find project ref for PR testing",
				"message": "project ref is disabled, not PR testing",
				"owner":   owner,
				"repo":    repo,
				"branch":  branch,
			})
			return nil, nil
		}
		if p.IsHidden() {
			hiddenProject = &projectRefs[i]
		}
	}
	if hiddenProject == nil {
		grip.Debug(message.Fields{
			"source":  "find project ref for PR testing",
			"message": "creating hidden project because none exists",
			"owner":   owner,
			"repo":    repo,
			"branch":  branch,
		})
		// if no project exists, create and return skeleton project
		hiddenProject = &ProjectRef{
			Id:              mgobson.NewObjectId().Hex(),
			Owner:           owner,
			Repo:            repo,
			Branch:          branch,
			RepoRefId:       repoRef.Id,
			UseRepoSettings: true,
			Enabled:         utility.FalsePtr(),
			Hidden:          utility.TruePtr(),
		}
		if err = hiddenProject.Add(nil); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "find project ref for PR testing",
				"message": "hidden project could not be added",
				"owner":   owner,
				"repo":    repo,
				"branch":  branch,
			}))
			return nil, nil
		}
	}

	return hiddenProject, nil
}

// FindBranchProjectRef finds the project ref for this owner/repo/branch that has the commit queue enabled.
// There should only ever be one project for the query because we only enable commit queue if
// no other project ref with the same specification has it enabled.

func FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}
	for _, p := range projectRefs {
		if p.CommitQueue.IsEnabled() {
			p.checkDefaultLogger()
			return &p, nil
		}
	}

	grip.Debug(message.Fields{
		"message": "no matching project ref with commit queue enabled",
		"owner":   owner,
		"repo":    repo,
		"branch":  branch,
	})
	return nil, nil
}

func FindHiddenProjectRefByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	q := byOwnerRepoAndBranch(owner, repo, branch)
	q[ProjectRefHiddenKey] = true

	return findOneProjectRefQ(db.Query(q))
}

func FindMergedEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	pipeline := []bson.M{{"$match": byOwnerAndRepo(owner, repo)}}
	pipeline = append(pipeline, projectRefPipelineForValueIsBool(ProjectRefEnabledKey, RepoRefEnabledKey, true)...)
	err := db.Aggregate(ProjectRefCollection, pipeline, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

// FindMergedProjectRefsForRepo considers either owner/repo and repo ref ID, in case the owner/repo of the repo ref is going to change.
// So we get all the branch projects in the new repo, and all the branch projects that might change owner/repo.
func FindMergedProjectRefsForRepo(repoRef *RepoRef) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	q := db.Query(bson.M{
		"$or": []bson.M{
			{
				ProjectRefOwnerKey: repoRef.Owner,
				ProjectRefRepoKey:  repoRef.Repo,
			},
			{ProjectRefRepoRefIdKey: repoRef.Id},
		},
	})
	err := db.FindAllQ(ProjectRefCollection, q, &projectRefs)
	if err != nil {
		return nil, err
	}

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
		if projectRefs[i].UseRepoSettings {
			mergedProject, err := mergeBranchAndRepoSettings(&projectRefs[i], repoRef)
			if err != nil {
				return nil, errors.Wrapf(err, "error merging settings")
			}
			projectRefs[i] = *mergedProject
		}
	}
	return projectRefs, nil
}

func GetProjectSettingsById(projectId string, isRepo bool) (*ProjectSettings, error) {
	var pRef *ProjectRef
	var err error
	if isRepo {
		repoRef, err := FindOneRepoRef(projectId)
		if err != nil {
			return nil, errors.Wrapf(err, "error finding repo ref")
		}
		if repoRef == nil {
			return nil, errors.Wrap(err, "couldn't find repo ref")
		}
		pRef = &repoRef.ProjectRef
	} else {
		pRef, err = FindBranchProjectRef(projectId)

	}
	if err != nil {
		return nil, errors.Wrapf(err, "error finding project ref")
	}
	if pRef == nil {
		return nil, errors.Errorf("couldn't find project ref")
	}
	return GetProjectSettings(pRef)
}

func GetProjectSettings(p *ProjectRef) (*ProjectSettings, error) {
	hook, err := FindGithubHook(p.Owner, p.Repo)
	if err != nil {
		return nil, errors.Wrapf(err, "Database error finding github hook for project '%s'", p.Id)
	}
	projectVars, err := FindOneProjectVars(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding variables for project '%s'", p.Id)
	}
	if projectVars == nil {
		projectVars = &ProjectVars{}
	}
	projectAliases, err := FindAliasesForProject(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding aliases for project '%s'", p.Id)
	}
	subscriptions, err := event.FindSubscriptionsByOwner(p.Id, event.OwnerTypeProject)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding subscription for project '%s'", p.Id)
	}
	projectSettingsEvent := ProjectSettings{
		ProjectRef:         *p,
		GitHubHooksEnabled: hook != nil,
		Vars:               *projectVars,
		Aliases:            projectAliases,
		Subscriptions:      subscriptions,
	}
	return &projectSettingsEvent, nil
}

func IsPerfEnabledForProject(projectId string) bool {
	projectRef, err := FindMergedProjectRef(projectId)
	if err != nil || projectRef == nil {
		return false
	}
	err = projectRef.MergeWithParserProject("")
	if err != nil {
		return false
	}
	return projectRef.IsPerfEnabled()
}

func UpdateOwnerAndRepoForBranchProjects(repoId, owner, repo string) error {
	return db.Update(
		ProjectRefCollection,
		bson.M{
			ProjectRefRepoRefIdKey:       repoId,
			projectRefUseRepoSettingsKey: true,
		},
		bson.M{
			"$set": bson.M{
				ProjectRefOwnerKey: owner,
				ProjectRefRepoKey:  repo,
			},
		})
}

func FindProjectRefsWithCommitQueueEnabled() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.Aggregate(
		ProjectRefCollection,
		projectRefPipelineForCommitQueueEnabled(),
		&projectRefs)
	if err != nil {
		return nil, err
	}

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, nil
}

func FindPeriodicProjects() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.Aggregate(
		ProjectRefCollection,
		projectRefPipelineForPeriodicBuilds(),
		&projectRefs,
	)
	if err != nil {
		return nil, err
	}

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, nil
}

// FindProjectRefs returns limit refs starting at project id key in the sortDir direction.
func FindProjectRefs(key string, limit int, sortDir int) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	filter := bson.M{}
	sortSpec := ProjectRefIdKey

	if sortDir < 0 {
		sortSpec = "-" + sortSpec
		filter[ProjectRefIdKey] = bson.M{"$lt": key}
	} else {
		filter[ProjectRefIdKey] = bson.M{"$gte": key}
	}

	q := db.Query(filter).Sort([]string{sortSpec}).Limit(limit)
	err := db.FindAllQ(ProjectRefCollection, q, &projectRefs)

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, err
}

func (projectRef *ProjectRef) CanEnableCommitQueue() (bool, error) {
	resultRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return false, errors.Wrapf(err, "database error finding project by repo and branch")
	}
	if resultRef != nil && resultRef.Id != projectRef.Id {
		return false, nil
	}
	return true, nil
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, one is created
func (projectRef *ProjectRef) Upsert() error {
	_, err := db.Upsert(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdKey: projectRef.Id,
		}, projectRef)
	return err
}

// SaveProjectPageForSection updates the project or repo ref variables for the section (if no project is given, we unset to default to repo).
func SaveProjectPageForSection(projectId string, p *ProjectRef, section ProjectPageSection, isRepo bool) (bool, error) {
	coll := ProjectRefCollection
	if isRepo {
		coll = RepoRefCollection
		if p == nil {
			return false, errors.New("can't default repo to repo")
		}
	}
	if p == nil {
		p = &ProjectRef{} // use a blank project ref to default the section to repo
	}
	var err error
	switch section {
	case ProjectPageGeneralSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					ProjectRefEnabledKey:                 p.Enabled,
					ProjectRefBatchTimeKey:               p.BatchTime,
					ProjectRefRemotePathKey:              p.RemotePath,
					projectRefSpawnHostScriptPathKey:     p.SpawnHostScriptPath,
					projectRefDispatchingDisabledKey:     p.DispatchingDisabled,
					ProjectRefDeactivatePreviousKey:      p.DeactivatePrevious,
					projectRefRepotrackerDisabledKey:     p.RepotrackerDisabled,
					projectRefDefaultLoggerKey:           p.DefaultLogger,
					projectRefCedarTestResultsEnabledKey: p.CedarTestResultsEnabled,
					projectRefPatchingDisabledKey:        p.PatchingDisabled,
					projectRefTaskSyncKey:                p.TaskSync,
					ProjectRefDisabledStatsCacheKey:      p.DisabledStatsCache,
					ProjectRefFilesIgnoredFromCacheKey:   p.FilesIgnoredFromCache,
				},
			})
	case ProjectPageAccessSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					ProjectRefPrivateKey:    p.Private,
					ProjectRefRestrictedKey: p.Restricted,
					ProjectRefAdminsKey:     p.Admins,
				},
			})
	case ProjectPageGithubAndCQSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefPRTestingEnabledKey:      p.PRTestingEnabled,
					projectRefGithubChecksEnabledKey:   p.GithubChecksEnabled,
					projectRefGithubTriggerAliasesKey:  p.PatchTriggerAliases,
					projectRefGitTagVersionsEnabledKey: p.GitTagVersionsEnabled,
					ProjectRefGitTagAuthorizedUsersKey: p.GitTagAuthorizedUsers,
					ProjectRefGitTagAuthorizedTeamsKey: p.GitTagAuthorizedTeams,
					projectRefCommitQueueKey:           p.CommitQueue,
				},
			})
	case ProjectPageNotificationsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{projectRefNotifyOnFailureKey: p.NotifyOnBuildFailure},
			})
	case ProjectPageWorkstationsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{projectRefWorkstationConfigKey: p.WorkstationConfig},
			})
	case ProjectPageTriggersSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefTriggersKey: p.Triggers,
				},
			})

	// todo: add casing on Build Baron and task annotation settings once EVG-15218 is complete

	case ProjectPagePatchAliasSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefPatchTriggerAliasesKey: p.PatchTriggerAliases,
				},
			})
	case ProjectPagePeriodicBuildsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{projectRefPeriodicBuildsKey: p.PeriodicBuilds},
			})
	case ProjectPageVariablesSection:
		// this section doesn't modify the project/repo ref
		return false, nil
	default:
		return false, errors.Errorf("invalid section")
	}

	if err != nil {
		return false, errors.Wrap(err, "error saving section")
	}
	return true, nil
}

// DefaultSectionToRepo modifies a subset of the project ref to use the repo values instead.
// This subset is based on the pages used in Spruce.
// If project settings aren't given, we should assume we're defaulting to repo and we need
// to create our own project settings event  after completing the update.
func DefaultSectionToRepo(projectId string, section ProjectPageSection, userId string) error {
	before, err := GetProjectSettingsById(projectId, false)
	if err != nil {
		return errors.Wrap(err, "error getting before project settings event")
	}

	modified, err := SaveProjectPageForSection(projectId, nil, section, false)
	if err != nil {
		return errors.Wrapf(err, "error defaulting project ref to repo for section '%s'", section)
	}

	// Handle sections that modify collections outside of the project ref.
	// Handle errors at the end so that we can still log the project as modified, if applicable.
	catcher := grip.NewBasicCatcher()
	switch section {
	case ProjectPageVariablesSection:
		err = db.Update(ProjectVarsCollection,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$unset": bson.M{
					projectVarsMapKey:    1,
					privateVarsMapKey:    1,
					restrictedVarsMapKey: 1,
				},
			})
		if err == nil {
			modified = true
		}
		catcher.Wrapf(err, "error defaulting to repo for section '%s'", section)
	case ProjectPageGithubAndCQSection:
		for _, a := range before.Aliases {
			// remove only internal aliases; any alias without these labels is a patch alias
			if utility.StringSliceContains(evergreen.InternalAliases, a.Alias) {
				err = RemoveProjectAlias(a.ID.Hex())
				if err == nil {
					modified = true // track if any aliases here were correctly modified so we can log the changes
				}
				catcher.Add(err)
			}
		}
	case ProjectPageNotificationsSection:
		// handle subscriptions
		for _, sub := range before.Subscriptions {
			err = event.RemoveSubscription(sub.ID)
			if err == nil {
				modified = true // track if any subscriptions were correctly modified so we can log the changes
			}
			catcher.Add(err)
		}
	case ProjectPagePatchAliasSection:
		catcher := grip.NewBasicCatcher()
		// remove only patch aliases, i.e. aliases without an Evergreen-internal label
		for _, a := range before.Aliases {
			if !utility.StringSliceContains(evergreen.InternalAliases, a.Alias) {
				err = RemoveProjectAlias(a.ID.Hex())
				if err == nil {
					modified = true // track if any aliases were correctly modified so we can log the changes
				}
				catcher.Add(err)
			}
		}
	}
	if modified {
		catcher.Add(GetAndLogProjectModified(projectId, userId, false, before))
	}

	return errors.Wrapf(catcher.Resolve(), "error defaulting to repo for section '%s'", section)
}

// getBatchTimeForVariant returns the Batch Time to be used for this variant
func (p *ProjectRef) getBatchTimeForVariant(variant *BuildVariant) int {
	val := p.BatchTime
	if variant.BatchTime != nil {
		val = *variant.BatchTime
	}
	return handleBatchTimeOverflow(val)
}

func (p *ProjectRef) getBatchTimeForTask(t *BuildVariantTaskUnit) int {
	val := p.BatchTime
	if t.BatchTime != nil {
		val = *t.BatchTime
	}
	return handleBatchTimeOverflow(val)
}

// BatchTime is in minutes, but it is stored/used internally as
// nanoseconds. We need to cap this value to prevent an
// overflow/wrap around to negative values of time.Duration
func handleBatchTimeOverflow(in int) int {
	if in > maxBatchTime {
		return maxBatchTime
	}
	return in
}

// return the next valid batch time
func GetActivationTimeWithCron(curTime time.Time, cronBatchTime string) (time.Time, error) {

	if strings.HasPrefix(cronBatchTime, intervalPrefix) {
		return time.Time{}, errors.Errorf("cannot use '%s' in cron batchtime '%s'", intervalPrefix, cronBatchTime)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
	sched, err := parser.Parse(cronBatchTime)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "error parsing cron batchtime '%s'", cronBatchTime)
	}
	return sched.Next(curTime), nil
}

func (p *ProjectRef) GetActivationTimeForVariant(variant *BuildVariant) (time.Time, error) {
	defaultRes := time.Now()
	// if we don't want to activate the build, set batchtime to the zero time
	if !utility.FromBoolTPtr(variant.Activate) {
		return utility.ZeroTime, nil
	}
	if variant.CronBatchTime != "" {
		return GetActivationTimeWithCron(time.Now(), variant.CronBatchTime)
	}
	// if activated explicitly set to true and we don't have batchtime, then we want to just activate now
	if utility.FromBoolPtr(variant.Activate) && variant.BatchTime == nil {
		return time.Now(), nil
	}

	lastActivated, err := VersionFindOne(VersionByLastVariantActivation(p.Id, variant.Name).WithFields(VersionBuildVariantsKey))
	if err != nil {
		return time.Time{}, errors.Wrap(err, "error finding version")
	}

	if lastActivated == nil {
		return defaultRes, nil
	}

	// find matching activated build variant
	for _, buildStatus := range lastActivated.BuildVariants {
		if buildStatus.BuildVariant != variant.Name || !buildStatus.Activated {
			continue
		}

		return buildStatus.ActivateAt.Add(time.Minute * time.Duration(p.getBatchTimeForVariant(variant))), nil
	}

	return defaultRes, nil
}

func (p *ProjectRef) GetActivationTimeForTask(t *BuildVariantTaskUnit) (time.Time, error) {
	defaultRes := time.Now()
	// if we don't want to activate the task, set batchtime to the zero time
	if !utility.FromBoolTPtr(t.Activate) {
		return utility.ZeroTime, nil
	}
	if t.CronBatchTime != "" {
		return GetActivationTimeWithCron(time.Now(), t.CronBatchTime)
	}
	// if activated explicitly set to true and we don't have batchtime, then we want to just activate now
	if utility.FromBoolPtr(t.Activate) && t.BatchTime == nil {
		return time.Now(), nil
	}

	lastActivated, err := VersionFindOne(VersionByLastTaskActivation(p.Id, t.Variant, t.Name).WithFields(VersionBuildVariantsKey))
	if err != nil {
		return defaultRes, errors.Wrap(err, "error finding version")
	}
	if lastActivated == nil {
		return defaultRes, nil
	}

	for _, buildStatus := range lastActivated.BuildVariants {
		// don't check buildStatus activation; this corresponds to the batchtime for the overall variant, not the individual tasks.
		if buildStatus.BuildVariant != t.Variant {
			continue
		}
		for _, taskStatus := range buildStatus.BatchTimeTasks {
			if taskStatus.TaskName != t.Name || !taskStatus.Activated {
				continue
			}
			return taskStatus.ActivateAt.Add(time.Minute * time.Duration(p.getBatchTimeForTask(t))), nil
		}
	}
	return defaultRes, nil
}

func (p *ProjectRef) ValidateOwnerAndRepo(validOrgs []string) error {
	// verify input and webhooks
	if p.Owner == "" || p.Repo == "" {
		return errors.New("no owner/repo specified")
	}

	if len(validOrgs) > 0 && !utility.StringSliceContains(validOrgs, p.Owner) {
		return errors.New("owner not authorized")
	}
	return nil
}

func (p *ProjectRef) ValidateIdentifier() error {
	if p.Id == p.Identifier { // we already know the id is unique
		return nil
	}
	count, err := CountProjectRefsWithIdentifier(p.Identifier)
	if err != nil {
		return errors.Wrapf(err, "error counting other project refs")
	}
	if count > 0 {
		return errors.New("identifier cannot match another project's identifier")
	}
	return nil
}

// RemoveAdminFromProjects removes a user from all Admin slices of every project and repo
func RemoveAdminFromProjects(toDelete string) error {
	projectUpdate := bson.M{
		"$pull": bson.M{
			ProjectRefAdminsKey: toDelete,
		},
	}
	repoUpdate := bson.M{
		"$pull": bson.M{
			RepoRefAdminsKey: toDelete,
		},
	}

	catcher := grip.NewBasicCatcher()
	_, err := db.UpdateAll(ProjectRefCollection, bson.M{ProjectRefAdminsKey: bson.M{"$ne": nil}}, projectUpdate)
	catcher.Add(errors.Wrap(err, "error updating projects"))
	_, err = db.UpdateAll(RepoRefCollection, bson.M{RepoRefAdminsKey: bson.M{"$ne": nil}}, repoUpdate)
	catcher.Add(errors.Wrap(err, "error updating repos"))
	return catcher.Resolve()
}

func (p *ProjectRef) MakeRestricted() error {
	rm := evergreen.GetEnvironment().RoleManager()
	// remove from the unrestricted branch project scope (if it exists)
	if p.UseRepoSettings {
		scopeId := GetUnrestrictedBranchProjectsScope(p.RepoRefId)
		if err := rm.RemoveResourceFromScope(scopeId, p.Id); err != nil {
			return errors.Wrap(err, "error removing resource from unrestricted branches scope")
		}
	}

	if err := rm.RemoveResourceFromScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "unable to remove %s from list of unrestricted projects", p.Id)
	}
	if err := rm.AddResourceToScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "unable to add %s to list of restricted projects", p.Id)
	}

	return nil
}

func (p *ProjectRef) MakeUnrestricted() error {
	rm := evergreen.GetEnvironment().RoleManager()
	// remove from the unrestricted branch project scope (if it exists)
	if p.UseRepoSettings {
		scopeId := GetUnrestrictedBranchProjectsScope(p.RepoRefId)
		if err := rm.AddResourceToScope(scopeId, p.Id); err != nil {
			return errors.Wrap(err, "error adding resource to unrestricted branches scope")
		}
	}

	if err := rm.RemoveResourceFromScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "unable to remove %s from list of restricted projects", p.Id)
	}
	if err := rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "unable to add %s to list of unrestricted projects", p.Id)
	}
	return nil
}

func (p *ProjectRef) UpdateAdminRoles(toAdd, toRemove []string) error {
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return nil
	}
	rm := evergreen.GetEnvironment().RoleManager()
	role, err := rm.FindRoleWithPermissions(evergreen.ProjectResourceType, []string{p.Id}, adminPermissions)
	if err != nil {
		return errors.Wrap(err, "error finding role with admin permissions")
	}
	if role == nil {
		return errors.Errorf("no admin role for %s found", p.Id)
	}
	viewRole := ""
	allBranchAdmins := []string{}
	if p.RepoRefId != "" {
		allBranchAdmins, err = FindBranchAdminsForRepo(p.RepoRefId)
		if err != nil {
			return errors.Wrapf(err, "error finding branch admins for repo '%s'", p.RepoRefId)
		}
		viewRole = GetViewRepoRole(p.RepoRefId)
	}

	catcher := grip.NewBasicCatcher()
	for _, addedUser := range toAdd {
		adminUser, err := user.FindOneById(addedUser)
		if err != nil {
			catcher.Wrapf(err, "error finding user '%s'", addedUser)
			p.removeFromAdminsList(addedUser)
			continue
		}
		if adminUser == nil {
			catcher.Errorf("no user '%s' found", addedUser)
			p.removeFromAdminsList(addedUser)
			continue
		}
		if err = adminUser.AddRole(role.ID); err != nil {
			catcher.Wrapf(err, "error adding role %s to user %s", role.ID, addedUser)
			p.removeFromAdminsList(addedUser)
			continue
		}
		if viewRole != "" {
			if err = adminUser.AddRole(viewRole); err != nil {
				catcher.Wrapf(err, "error adding role %s to user %s", viewRole, addedUser)
				continue
			}
		}
	}
	for _, removedUser := range toRemove {
		adminUser, err := user.FindOneById(removedUser)
		if err != nil {
			catcher.Wrapf(err, "error finding user %s", removedUser)
			continue
		}
		if adminUser == nil {
			continue
		}

		if err = adminUser.RemoveRole(role.ID); err != nil {
			catcher.Wrapf(err, "error removing role %s from user %s", role.ID, removedUser)
			p.Admins = append(p.Admins, removedUser)
			continue
		}
		if viewRole != "" && !utility.StringSliceContains(allBranchAdmins, adminUser.Id) {
			if err = adminUser.RemoveRole(viewRole); err != nil {
				catcher.Wrapf(err, "error removing role %s from user %s", viewRole, removedUser)
				continue
			}
		}
	}
	if err = catcher.Resolve(); err != nil {
		return errors.Wrap(err, "error updating some admins")
	}
	return nil
}

func (p *ProjectRef) removeFromAdminsList(user string) {
	for i, name := range p.Admins {
		if name == user {
			p.Admins = append(p.Admins[:i], p.Admins[i+1:]...)
		}
	}
}

func (p *ProjectRef) AuthorizedForGitTag(ctx context.Context, githubUser string, token string) bool {
	if utility.StringSliceContains(p.GitTagAuthorizedUsers, githubUser) {
		return true
	}
	// check if user has permissions with mana before asking github about the teams
	u, err := user.FindByGithubName(githubUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error checking if user is authorized for git tag",
			"source":  "github hook",
		}))
	}
	if u != nil {
		hasPermission := u.HasPermission(gimlet.PermissionOpts{
			Resource:      p.Id,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionGitTagVersions,
			RequiredLevel: evergreen.GitTagVersionsCreate.Value,
		})
		if hasPermission {
			return true
		}
	}

	return thirdparty.IsUserInGithubTeam(ctx, p.GitTagAuthorizedTeams, p.Owner, githubUser, token)
}

// GetProjectSetupCommands returns jasper commands for the project's configuration commands
// Stderr/Stdin are passed through to the commands as well as Stdout, when opts.Quiet is false
// The commands' working directories may not exist and need to be created before running the commands
func (p *ProjectRef) GetProjectSetupCommands(opts apimodels.WorkstationSetupCommandOptions) ([]*jasper.Command, error) {
	if len(p.WorkstationConfig.SetupCommands) == 0 && !p.WorkstationConfig.ShouldGitClone() {
		return nil, errors.Errorf("no setup commands configured for project '%s'", p.Id)
	}

	baseDir := filepath.Join(opts.Directory, p.Id)
	cmds := []*jasper.Command{}

	if p.WorkstationConfig.ShouldGitClone() {
		args := []string{"git", "clone", "-b", p.Branch, fmt.Sprintf("git@github.com:%s/%s.git", p.Owner, p.Repo), opts.Directory}

		cmd := jasper.NewCommand().Add(args).
			SetErrorWriter(utility.NopWriteCloser(os.Stderr)).
			Prerequisite(func() bool {
				grip.Info(message.Fields{
					"directory": opts.Directory,
					"command":   strings.Join(args, " "),
					"op":        "repo clone",
					"project":   p.Id,
				})

				return !opts.DryRun
			})

		if !opts.Quiet {
			cmd = cmd.SetOutputWriter(utility.NopWriteCloser(os.Stdout))
		}

		cmds = append(cmds, cmd)

	}

	for idx, obj := range p.WorkstationConfig.SetupCommands {
		dir := baseDir
		if obj.Directory != "" {
			dir = filepath.Join(dir, obj.Directory)
		}

		// avoid logging the final value of obj
		commandNumber := idx + 1
		cmdString := obj.Command

		cmd := jasper.NewCommand().Directory(dir).SetErrorWriter(utility.NopWriteCloser(os.Stderr)).SetInput(os.Stdin).
			Append(obj.Command).
			Prerequisite(func() bool {
				grip.Info(message.Fields{
					"directory":      dir,
					"command":        cmdString,
					"command_number": commandNumber,
					"op":             "setup command",
					"project":        p.Id,
				})

				return !opts.DryRun
			})
		if !opts.Quiet {
			cmd = cmd.SetOutputWriter(utility.NopWriteCloser(os.Stdout))
		}
		cmds = append(cmds, cmd)
	}

	return cmds, nil
}

func (p *ProjectRef) UpdateNextPeriodicBuild(definition string, nextRun time.Time) error {
	for i, d := range p.PeriodicBuilds {
		if d.ID == definition {
			d.NextRunTime = nextRun
			p.PeriodicBuilds[i] = d
			break
		}
	}
	collection := ProjectRefCollection
	idKey := ProjectRefIdKey
	buildsKey := projectRefPeriodicBuildsKey
	if p.UseRepoSettings {
		// if the periodic build is part of the repo then update there instead
		repoRef, err := FindOneRepoRef(p.RepoRefId)
		if err != nil {
			return err
		}
		if repoRef == nil {
			return errors.New("couldn't find repo")
		}
		for _, d := range repoRef.PeriodicBuilds {
			if d.ID == definition {
				collection = RepoRefCollection
				idKey = RepoRefIdKey
				buildsKey = RepoRefPeriodicBuildsKey
			}
		}

	}

	filter := bson.M{
		idKey: p.Id,
		buildsKey: bson.M{
			"$elemMatch": bson.M{
				"id": definition,
			},
		},
	}
	update := bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(buildsKey, "$", "next_run_time"): nextRun,
		},
	}

	return errors.Wrapf(db.Update(collection, filter, update), "error updating collection '%s'", collection)
}

func (p *ProjectRef) CommitQueueIsOn() error {
	catcher := grip.NewBasicCatcher()
	if !p.IsEnabled() {
		catcher.Add(errors.Errorf("project '%s' is disabled", p.Id))
	}
	if p.IsPatchingDisabled() {
		catcher.Add(errors.Errorf("patching is disabled for project '%s'", p.Id))
	}
	if !p.CommitQueue.IsEnabled() {
		catcher.Add(errors.Errorf("commit queue is disabled for project '%s'", p.Id))
	}

	return catcher.Resolve()
}

func GetProjectRefForTask(taskId string) (*ProjectRef, error) {
	projectId, err := task.FindProjectForTask(taskId)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project")
	}
	pRef, err := FindMergedProjectRef(projectId)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting project '%s'", projectId)
	}
	if pRef == nil {
		return nil, errors.Errorf("project ref '%s' doesn't exist", projectId)
	}
	return pRef, nil
}

func GetSetupScriptForTask(ctx context.Context, taskId string) (string, error) {
	conf, err := evergreen.GetConfig()
	if err != nil {
		return "", errors.Wrap(err, "can't get evergreen configuration")
	}
	token, err := conf.GetGithubOauthToken()
	if err != nil {
		return "", errors.Wrap(err, "error getting github token")
	}

	pRef, err := GetProjectRefForTask(taskId)
	if err != nil {
		return "", errors.Wrap(err, "error getting project")
	}

	configFile, err := thirdparty.GetGithubFile(ctx, token, pRef.Owner, pRef.Repo, pRef.SpawnHostScriptPath, pRef.Branch)
	if err != nil {
		return "", errors.Wrapf(err,
			"error fetching spawn host script for '%s' at path '%s'", pRef.Identifier, pRef.SpawnHostScriptPath)
	}
	fileContents, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return "", errors.Wrapf(err,
			"unable to spawn host script for '%s' at path '%s'", pRef.Identifier, pRef.SpawnHostScriptPath)
	}

	return string(fileContents), nil
}

func (t TriggerDefinition) Validate(parentProject string) error {
	upstreamProject, err := FindBranchProjectRef(t.Project)
	if err != nil {
		return errors.Wrapf(err, "error finding upstream project %s", t.Project)
	}
	if upstreamProject == nil {
		return errors.Errorf("project '%s' not found", t.Project)
	}
	if upstreamProject.Id == parentProject {
		return errors.New("a project cannot trigger itself")
	}
	if t.Level != ProjectTriggerLevelBuild && t.Level != ProjectTriggerLevelTask {
		return errors.Errorf("invalid level: %s", t.Level)
	}
	if t.Status != "" && t.Status != evergreen.TaskFailed && t.Status != evergreen.TaskSucceeded {
		return errors.Errorf("invalid status: %s", t.Status)
	}
	_, regexErr := regexp.Compile(t.BuildVariantRegex)
	if regexErr != nil {
		return errors.Wrap(regexErr, "invalid variant regex")
	}
	_, regexErr = regexp.Compile(t.TaskRegex)
	if regexErr != nil {
		return errors.Wrap(regexErr, "invalid task regex")
	}
	if t.ConfigFile == "" && t.GenerateFile == "" {
		return errors.New("must provide a config file or generated tasks file")
	}
	return nil
}

func ValidateTriggerDefinition(definition patch.PatchTriggerDefinition, parentProject string) (patch.PatchTriggerDefinition, error) {
	if definition.ChildProject == parentProject {
		return definition, errors.New("a project cannot trigger itself")
	}

	childProjectId, err := GetIdForProject(definition.ChildProject)
	if err != nil {
		return definition, errors.Wrapf(err, "error finding child project '%s'", definition.ChildProject)
	}

	if !utility.StringSliceContains([]string{"", AllStatuses, evergreen.PatchSucceeded, evergreen.PatchFailed}, definition.Status) {
		return definition, errors.Errorf("invalid status: %s", definition.Status)
	}

	// ChildProject should be saved using its ID, in case the user used the project's Identifier
	definition.ChildProject = childProjectId

	for _, specifier := range definition.TaskSpecifiers {
		if (specifier.VariantRegex != "" || specifier.TaskRegex != "") && specifier.PatchAlias != "" {
			return definition, errors.New("can't specify both a regex set and a patch alias")
		}

		if specifier.PatchAlias == "" && (specifier.TaskRegex == "" || specifier.VariantRegex == "") {
			return definition, errors.New("must specify either a patch alias or a complete regex set")
		}

		if specifier.VariantRegex != "" {
			_, regexErr := regexp.Compile(specifier.VariantRegex)
			if regexErr != nil {
				return definition, errors.Wrapf(regexErr, "invalid variant regex '%s'", specifier.VariantRegex)
			}
		}

		if specifier.TaskRegex != "" {
			_, regexErr := regexp.Compile(specifier.TaskRegex)
			if regexErr != nil {
				return definition, errors.Wrapf(regexErr, "invalid task regex '%s'", specifier.TaskRegex)
			}
		}

		if specifier.PatchAlias != "" {
			var aliases []ProjectAlias
			aliases, err = FindAliasInProjectOrRepo(definition.ChildProject, specifier.PatchAlias)
			if err != nil {
				return definition, errors.Wrap(err, "problem fetching aliases for project")
			}
			if len(aliases) == 0 {
				return definition, errors.Errorf("patch alias '%s' is not defined for project '%s'", specifier.PatchAlias, definition.ChildProject)
			}
		}
	}

	return definition, nil
}

func (d *PeriodicBuildDefinition) Validate() error {
	catcher := grip.NewBasicCatcher()
	if d.IntervalHours <= 0 {
		catcher.New("Interval must be a positive integer")
	}
	if d.ConfigFile == "" {
		catcher.New("A config file must be specified")
	}
	if d.Alias == "" {
		catcher.New("Alias must be specified")
	}

	if d.ID == "" {
		d.ID = utility.RandomString()
	}

	return catcher.Resolve()
}

func GetUpstreamProjectName(triggerID, triggerType string) (string, error) {
	if triggerID == "" || triggerType == "" {
		return "", nil
	}
	var projectID string
	if triggerType == ProjectTriggerLevelTask {
		upstreamTask, err := task.FindOneId(triggerID)
		if err != nil {
			return "", errors.Wrap(err, "error finding upstream task")
		}
		if upstreamTask == nil {
			return "", errors.New("upstream task not found")
		}
		projectID = upstreamTask.Project
	} else if triggerType == ProjectTriggerLevelBuild {
		upstreamBuild, err := build.FindOneId(triggerID)
		if err != nil {
			return "", errors.Wrap(err, "error finding upstream build")
		}
		if upstreamBuild == nil {
			return "", errors.New("upstream build not found")
		}
		projectID = upstreamBuild.Project
	}
	upstreamProject, err := FindBranchProjectRef(projectID)
	if err != nil {
		return "", errors.Wrap(err, "error finding upstream project")
	}
	if upstreamProject == nil {
		return "", errors.New("upstream project not found")
	}
	return upstreamProject.DisplayName, nil
}

// projectRefPipelineForMatchingTrigger is an aggregation pipeline to find projects that have the projectKey
// explicitly set to the val, OR that default to the repo, which has the repoKey explicitly set to the val
func projectRefPipelineForValueIsBool(projectKey, repoKey string, val bool) []bson.M {
	return []bson.M{
		lookupRepoStep,
		{"$match": bson.M{
			"$or": []bson.M{
				{projectKey: val},
				{projectKey: nil, bsonutil.GetDottedKeyName("repo_ref", repoKey): val},
			},
		}},
	}
}

// projectRefPipelineForMatchingTrigger is an aggregation pipeline to find projects that are
// 1) explicitly enabled, or that default to the repo which is enabled, and
// 2) they have triggers defined for this project, or they default to the repo, which has a trigger for this project defined.
func projectRefPipelineForMatchingTrigger(project string) []bson.M {
	return []bson.M{
		lookupRepoStep,
		{"$match": bson.M{
			"$and": []bson.M{
				{"$or": []bson.M{
					{ProjectRefEnabledKey: true},
					{ProjectRefEnabledKey: bson.M{"$ne": false}, bsonutil.GetDottedKeyName("repo_ref", RepoRefEnabledKey): true},
				}},
				{"$or": []bson.M{
					{
						bsonutil.GetDottedKeyName(projectRefTriggersKey, triggerDefinitionProjectKey): project,
					},
					{
						projectRefTriggersKey: nil,
						bsonutil.GetDottedKeyName("repo_ref", RepoRefTriggersKey, triggerDefinitionProjectKey): project,
					},
				}},
			}},
		},
	}
}

// projectRefPipelineForCommitQueue is an aggregation pipeline to find projects that are
// 1) explicitly enabled, or that default to the repo which is enabled, and
// 2) the commit queue is explicitly enabled, or defaults to the repo which has the commit queue enabled
func projectRefPipelineForCommitQueueEnabled() []bson.M {
	return []bson.M{
		lookupRepoStep,
		{"$match": bson.M{
			"$and": []bson.M{
				{"$or": []bson.M{
					{ProjectRefEnabledKey: true},
					{ProjectRefEnabledKey: nil, bsonutil.GetDottedKeyName("repo_ref", RepoRefEnabledKey): true},
				}},
				{"$or": []bson.M{
					{
						bsonutil.GetDottedKeyName(projectRefCommitQueueKey, commitQueueEnabledKey): true,
					},
					{
						bsonutil.GetDottedKeyName(projectRefCommitQueueKey, commitQueueEnabledKey):          nil,
						bsonutil.GetDottedKeyName("repo_ref", RepoRefCommitQueueKey, commitQueueEnabledKey): true,
					},
				}},
			}},
		},
	}
}

// projectRefPipelineForPeriodicBuilds is an aggregation pipeline to find projects that are
// 1) explicitly enabled, or that default to the repo which is enabled, and
// 2) they have periodic builds defined, or they default to the repo which has periodic builds defined.
func projectRefPipelineForPeriodicBuilds() []bson.M {
	nonEmptySize := bson.M{"$gt": bson.M{"$size": 0}}
	return []bson.M{
		lookupRepoStep,
		{"$match": bson.M{
			"$and": []bson.M{
				{"$or": []bson.M{
					{ProjectRefEnabledKey: true},
					{ProjectRefEnabledKey: nil, bsonutil.GetDottedKeyName("repo_ref", RepoRefEnabledKey): true},
				}},
				{"$or": []bson.M{
					{
						projectRefPeriodicBuildsKey: nonEmptySize,
					},
					{
						projectRefPeriodicBuildsKey:                                     nil,
						bsonutil.GetDottedKeyName("repo_ref", RepoRefPeriodicBuildsKey): nonEmptySize,
					},
				}},
			}},
		},
	}
}

var lookupRepoStep = bson.M{"$lookup": bson.M{
	"from":         RepoRefCollection,
	"localField":   ProjectRefRepoRefIdKey,
	"foreignField": RepoRefIdKey,
	"as":           "repo_ref",
}}
