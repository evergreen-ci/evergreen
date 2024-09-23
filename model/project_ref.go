package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"go.mongodb.org/mongo-driver/bson"
)

// ProjectRef contains Evergreen project-related settings which can be set
// independently of version control.
// Booleans that can be defined from both the repo and branch must be pointers, so that branch configurations can specify when to default to the repo.
type ProjectRef struct {
	// Id is the unmodifiable unique ID for the configuration, used internally.
	Id string `bson:"_id" json:"id" yaml:"id"`
	// Identifier must be unique, but is modifiable. Used by users.
	Identifier string `bson:"identifier" json:"identifier" yaml:"identifier"`

	// RemotePath is the path to the Evergreen config file.
	RemotePath             string              `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	DisplayName            string              `bson:"display_name" json:"display_name,omitempty" yaml:"display_name"`
	Enabled                bool                `bson:"enabled,omitempty" json:"enabled,omitempty" yaml:"enabled"`
	Restricted             *bool               `bson:"restricted,omitempty" json:"restricted,omitempty" yaml:"restricted"`
	Owner                  string              `bson:"owner_name" json:"owner_name" yaml:"owner"`
	Repo                   string              `bson:"repo_name" json:"repo_name" yaml:"repo"`
	Branch                 string              `bson:"branch_name" json:"branch_name" yaml:"branch"`
	PatchingDisabled       *bool               `bson:"patching_disabled,omitempty" json:"patching_disabled,omitempty"`
	RepotrackerDisabled    *bool               `bson:"repotracker_disabled,omitempty" json:"repotracker_disabled,omitempty" yaml:"repotracker_disabled"`
	DispatchingDisabled    *bool               `bson:"dispatching_disabled,omitempty" json:"dispatching_disabled,omitempty" yaml:"dispatching_disabled"`
	StepbackDisabled       *bool               `bson:"stepback_disabled,omitempty" json:"stepback_disabled,omitempty" yaml:"stepback_disabled"`
	StepbackBisect         *bool               `bson:"stepback_bisect,omitempty" json:"stepback_bisect,omitempty" yaml:"stepback_bisect"`
	VersionControlEnabled  *bool               `bson:"version_control_enabled,omitempty" json:"version_control_enabled,omitempty" yaml:"version_control_enabled"`
	PRTestingEnabled       *bool               `bson:"pr_testing_enabled,omitempty" json:"pr_testing_enabled,omitempty" yaml:"pr_testing_enabled"`
	ManualPRTestingEnabled *bool               `bson:"manual_pr_testing_enabled,omitempty" json:"manual_pr_testing_enabled,omitempty" yaml:"manual_pr_testing_enabled"`
	GithubChecksEnabled    *bool               `bson:"github_checks_enabled,omitempty" json:"github_checks_enabled,omitempty" yaml:"github_checks_enabled"`
	BatchTime              int                 `bson:"batch_time" json:"batch_time" yaml:"batchtime"`
	DeactivatePrevious     *bool               `bson:"deactivate_previous,omitempty" json:"deactivate_previous,omitempty" yaml:"deactivate_previous"`
	NotifyOnBuildFailure   *bool               `bson:"notify_on_failure,omitempty" json:"notify_on_failure,omitempty"`
	Triggers               []TriggerDefinition `bson:"triggers" json:"triggers"`
	// all aliases defined for the project
	PatchTriggerAliases []patch.PatchTriggerDefinition `bson:"patch_trigger_aliases" json:"patch_trigger_aliases"`
	// all PatchTriggerAliases applied to github patch intents
	GithubTriggerAliases []string `bson:"github_trigger_aliases" json:"github_trigger_aliases"`
	// OldestAllowedMergeBase is the commit hash of the oldest merge base on the target branch
	// that PR patches can be created from.
	OldestAllowedMergeBase string                    `bson:"oldest_allowed_merge_base" json:"oldest_allowed_merge_base"`
	PeriodicBuilds         []PeriodicBuildDefinition `bson:"periodic_builds" json:"periodic_builds"`
	CommitQueue            CommitQueueParams         `bson:"commit_queue" json:"commit_queue" yaml:"commit_queue"`

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

	// Disable task stats caching for this project.
	DisabledStatsCache *bool `bson:"disabled_stats_cache,omitempty" json:"disabled_stats_cache,omitempty"`

	// List of commands
	// Lacks omitempty so that SetupCommands can be identified as either [] or nil in a ProjectSettingsEvent
	WorkstationConfig WorkstationConfig `bson:"workstation_config" json:"workstation_config"`

	// TaskAnnotationSettings holds settings for the file ticket button in the Task Annotations to call custom webhooks when clicked
	TaskAnnotationSettings evergreen.AnnotationsSettings `bson:"task_annotation_settings,omitempty" json:"task_annotation_settings,omitempty"`

	// Plugin settings
	BuildBaronSettings evergreen.BuildBaronSettings `bson:"build_baron_settings,omitempty" json:"build_baron_settings,omitempty" yaml:"build_baron_settings,omitempty"`
	PerfEnabled        *bool                        `bson:"perf_enabled,omitempty" json:"perf_enabled,omitempty" yaml:"perf_enabled,omitempty"`

	// Container settings
	ContainerSizeDefinitions []ContainerResources `bson:"container_size_definitions,omitempty" json:"container_size_definitions,omitempty" yaml:"container_size_definitions,omitempty"`
	ContainerSecrets         []ContainerSecret    `bson:"container_secrets,omitempty" json:"container_secrets,omitempty" yaml:"container_secrets,omitempty"`

	// RepoRefId is the repo ref id that this project ref tracks, if any.
	RepoRefId string `bson:"repo_ref_id" json:"repo_ref_id" yaml:"repo_ref_id"`

	// The following fields are used by Evergreen and are not discoverable.
	// Hidden determines whether or not the project is discoverable/tracked in the UI
	Hidden *bool `bson:"hidden,omitempty" json:"hidden,omitempty"`

	ExternalLinks []ExternalLink `bson:"external_links,omitempty" json:"external_links,omitempty" yaml:"external_links,omitempty"`
	Banner        ProjectBanner  `bson:"banner,omitempty" json:"banner,omitempty" yaml:"banner,omitempty"`

	// Filter/view settings
	ProjectHealthView ProjectHealthView `bson:"project_health_view" json:"project_health_view" yaml:"project_health_view"`
	ParsleyFilters    []parsley.Filter  `bson:"parsley_filters,omitempty" json:"parsley_filters,omitempty"`

	// GitHubDynamicTokenPermissionGroups is a list of permission groups for GitHub dynamic access tokens.
	GitHubDynamicTokenPermissionGroups []GitHubDynamicTokenPermissionGroup `bson:"github_dynamic_token_permission_groups,omitempty" json:"github_dynamic_token_permission_groups,omitempty" yaml:"github_dynamic_token_permission_groups,omitempty"`

	// GitHubPermissionGroupByRequester is a mapping of requester type to the user defined GitHub permission groups above.
	GitHubPermissionGroupByRequester map[string]string `bson:"github_token_permission_by_requester,omitempty" json:"github_token_permission_by_requester,omitempty" yaml:"github_token_permission_by_requester,omitempty"`

	// ParameterStoreEnabled is a temporary feature flag to enable/disable
	// Parameter Store for storing project secrets.
	ParameterStoreEnabled bool `bson:"parameter_store_enabled,omitempty" json:"parameter_store_enabled,omitempty" yaml:"parameter_store_enabled,omitempty"`
}

// GitHubDynamicTokenPermissionGroup is a permission group for GitHub dynamic access tokens.
type GitHubDynamicTokenPermissionGroup struct {
	// Name is the name of the group.
	Name string `bson:"name,omitempty" json:"name,omitempty" yaml:"name,omitempty"`
	// Permissions are a key-value pair of GitHub token permissions to their permission level
	Permissions github.InstallationPermissions `bson:"permissions,omitempty" json:"permissions,omitempty" yaml:"permissions,omitempty"`
	// AllPermissions is a flag that indicates that the group has all permissions.
	// If this is set to true, the Permissions field is ignored.
	// If this is set to false, the Permissions field is used (and may be
	// nil, representing no permissions).
	AllPermissions bool `bson:"all_permissions,omitempty" json:"all_permissions,omitempty" yaml:"all_permissions,omitempty"`
}

// defaultGitHubTokenPermissionGroup is an empty, all permissions, group.
var defaultGitHubTokenPermissionGroup = GitHubDynamicTokenPermissionGroup{
	AllPermissions: true,
}

// noPermissionsGitHubTokenPermissionGroup is an empty, no permissions, group.
var noPermissionsGitHubTokenPermissionGroup = GitHubDynamicTokenPermissionGroup{
	Name:           "No Permissions",
	AllPermissions: false,
}

// GetGitHubPermissionGroup returns the GitHubDynamicTokenPermissionGroup for the given requester.
// If the requester is not found, it returns the default permission group and a false boolean to
// indicate not found.
func (p *ProjectRef) GetGitHubPermissionGroup(requester string) (GitHubDynamicTokenPermissionGroup, bool) {
	if p.GitHubPermissionGroupByRequester == nil {
		return defaultGitHubTokenPermissionGroup, false
	}
	groupName, ok := p.GitHubPermissionGroupByRequester[requester]
	if !ok {
		return defaultGitHubTokenPermissionGroup, false
	}
	if groupName == noPermissionsGitHubTokenPermissionGroup.Name {
		return noPermissionsGitHubTokenPermissionGroup, true
	}
	for _, group := range p.GitHubDynamicTokenPermissionGroups {
		if group.Name == groupName {
			return group, true
		}
	}
	return defaultGitHubTokenPermissionGroup, false
}

func (p *ProjectRef) ValidateGitHubPermissionGroups() error {
	catcher := grip.NewBasicCatcher()
	for _, group := range p.GitHubDynamicTokenPermissionGroups {
		catcher.ErrorfWhen(group.Name == "", "group name cannot be empty")
	}
	for requester, groupName := range p.GitHubPermissionGroupByRequester {
		catcher.ErrorfWhen(
			!utility.StringSliceContains(evergreen.AllRequesterTypes, requester),
			"requester '%s' is not a valid requester", requester)

		_, found := p.GetGitHubPermissionGroup(requester)
		catcher.ErrorfWhen(
			!found,
			"group '%s' for requester '%s' not found", groupName, requester)
	}
	return errors.Wrap(catcher.Resolve(), "invalid GitHub dynamic token permission groups")
}

// Intersection returns the most restrictive intersection of the two permission groups.
// The name carries over from the calling group. If either permission is no permissions,
// it will return a group with no permissions.
func (p *GitHubDynamicTokenPermissionGroup) Intersection(other GitHubDynamicTokenPermissionGroup) (GitHubDynamicTokenPermissionGroup, error) {
	intersectionGroup := GitHubDynamicTokenPermissionGroup{Name: p.Name}
	if p.AllPermissions && other.AllPermissions {
		intersectionGroup.AllPermissions = true
		return intersectionGroup, nil
	}
	if p.AllPermissions {
		intersectionGroup.Permissions = other.Permissions
		return intersectionGroup, nil
	}
	if other.AllPermissions {
		intersectionGroup.Permissions = p.Permissions
		return intersectionGroup, nil
	}

	// To keep up to date with GitHub's different permissions,
	// we use reflection to iterate over the fields of the struct.

	// The two permissions to intersect.
	perms1 := reflect.ValueOf(&p.Permissions).Elem()
	perms2 := reflect.ValueOf(&other.Permissions).Elem()

	// The most restrictive intersection of the above permissions.
	intersection := reflect.ValueOf(&intersectionGroup.Permissions).Elem()

	// Iterate through all of their fields.
	for i := 0; i < perms1.NumField(); i++ {
		perm1Ptr, ok := perms1.Field(i).Interface().(*string)
		// This currently should not get triggered, but if GitHub
		// introduces a field that isn't a pointer to a string-
		// this will stop a wide spread panic.
		if !ok {
			continue
		}
		perm2Ptr, ok := perms2.Field(i).Interface().(*string)
		if !ok {
			continue
		}

		perm1 := utility.FromStringPtr(perm1Ptr)
		perm2 := utility.FromStringPtr(perm2Ptr)

		catcher := grip.NewBasicCatcher()
		catcher.Add(thirdparty.ValidateGitHubPermission(perm1))
		catcher.Add(thirdparty.ValidateGitHubPermission(perm2))
		if catcher.HasErrors() {
			return GitHubDynamicTokenPermissionGroup{}, catcher.Resolve()
		}

		mostRestrictivePermission := thirdparty.MostRestrictiveGitHubPermission(perm1, perm2)

		if mostRestrictivePermission == "" {
			continue
		}
		intersection.Field(i).Set(reflect.ValueOf(&mostRestrictivePermission))
	}

	return intersectionGroup, nil
}

// HasNoPermissions tests if the group has no permissions.
func (p *GitHubDynamicTokenPermissionGroup) HasNoPermissions() bool {
	if p.AllPermissions {
		return false
	}

	perms := reflect.ValueOf(&p.Permissions).Elem()
	for i := 0; i < perms.NumField(); i++ {
		permPtr, ok := perms.Field(i).Interface().(*string)
		if !ok {
			continue
		}
		perm := utility.FromStringPtr(permPtr)
		if perm != "" {
			return false
		}
	}

	return true
}

type ProjectHealthView string

const (
	ProjectHealthViewAll    ProjectHealthView = "ALL"
	ProjectHealthViewFailed ProjectHealthView = "FAILED"
)

type ProjectBanner struct {
	Theme evergreen.BannerTheme `bson:"theme" json:"theme"`
	Text  string                `bson:"text" json:"text"`
}

type ExternalLink struct {
	DisplayName string   `bson:"display_name,omitempty" json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Requesters  []string `bson:"requesters,omitempty" json:"requesters,omitempty" yaml:"requesters,omitempty"`
	URLTemplate string   `bson:"url_template,omitempty" json:"url_template,omitempty" yaml:"url_template,omitempty"`
}

type MergeQueue string

const (
	MergeQueueEvergreen MergeQueue = "EVERGREEN"
	MergeQueueGitHub    MergeQueue = "GITHUB"
)

type CommitQueueParams struct {
	Enabled     *bool      `bson:"enabled" json:"enabled" yaml:"enabled"`
	MergeMethod string     `bson:"merge_method" json:"merge_method" yaml:"merge_method"`
	MergeQueue  MergeQueue `bson:"merge_queue" json:"merge_queue" yaml:"merge_queue"`
	Message     string     `bson:"message,omitempty" json:"message,omitempty" yaml:"message"`
}

// TaskSyncOptions contains information about which features are allowed for
// syncing task directories to S3.
type TaskSyncOptions struct {
	ConfigEnabled *bool `bson:"config_enabled" json:"config_enabled" yaml:"config_enabled"`
	PatchEnabled  *bool `bson:"patch_enabled" json:"patch_enabled" yaml:"patch_enabled"`
}

// RepositoryErrorDetails indicates whether or not there is an invalid revision and if there is one,
// what the guessed merge base revision is.
type RepositoryErrorDetails struct {
	Exists            bool   `bson:"exists" json:"exists"`
	InvalidRevision   string `bson:"invalid_revision" json:"invalid_revision"`
	MergeBaseRevision string `bson:"merge_base_revision" json:"merge_base_revision"`
}

// ContainerResources specifies the computing resources given to the container.
// MemoryMB is the memory (in MB) that the container will be allocated, and
// CPU is the CPU units that will be allocated. 1024 CPU units is
// equivalent to 1vCPU.
type ContainerResources struct {
	Name     string `bson:"name,omitempty" json:"name" yaml:"name"`
	MemoryMB int    `bson:"memory_mb,omitempty" json:"memory_mb" yaml:"memory_mb"`
	CPU      int    `bson:"cpu,omitempty" json:"cpu" yaml:"cpu"`
}

// ContainerSecret specifies the username and password required for authentication
// on a private image repository. The credential is saved in AWS Secrets Manager upon
// saving to the ProjectRef
type ContainerSecret struct {
	// Name is the user-friendly display name of the secret.
	Name string `bson:"name" json:"name" yaml:"name"`
	// Type is the type of secret that is stored.
	Type ContainerSecretType `bson:"type" json:"type" yaml:"type"`
	// ExternalName is the name of the stored secret.
	ExternalName string `bson:"external_name" json:"external_name" yaml:"external_name"`
	// ExternalID is the unique resource identifier for the secret. This can be
	// used to access and modify the secret.
	ExternalID string `bson:"external_id" json:"external_id" yaml:"external_id"`
	// Value is the plaintext value of the secret. This is not stored and must
	// be retrieved using the external ID.
	Value string `bson:"-" json:"-" yaml:"-"`
}

// ContainerSecretType represents a particular type of container secret, which
// designates its purpose.
type ContainerSecretType string

const (
	// ContainerSecretPodSecret is a container secret representing the Evergreen
	// agent's pod secret.
	ContainerSecretPodSecret ContainerSecretType = "pod_secret"
	// ContainerSecretRepoCreds is a container secret representing an image
	// repository's credentials.
	ContainerSecretRepoCreds ContainerSecretType = "repository_credentials"
)

// Validate checks that the container secret type is recognized.
func (t ContainerSecretType) Validate() error {
	switch t {
	case ContainerSecretPodSecret, ContainerSecretRepoCreds:
		return nil
	default:
		return errors.Errorf("unrecognized container secret type '%s'", t)
	}
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
	ConfigFile                   string `bson:"config_file,omitempty" json:"config_file,omitempty"`
	Alias                        string `bson:"alias,omitempty" json:"alias,omitempty"`
	UnscheduleDownstreamVersions bool   `bson:"unschedule_downstream_versions,omitempty" json:"unschedule_downstream_versions,omitempty"`
}

type PeriodicBuildDefinition struct {
	ID            string    `bson:"id" json:"id"`
	ConfigFile    string    `bson:"config_file" json:"config_file"`
	IntervalHours int       `bson:"interval_hours" json:"interval_hours"`
	Cron          string    `bson:"cron" json:"cron"`
	Alias         string    `bson:"alias,omitempty" json:"alias,omitempty"`
	Message       string    `bson:"message,omitempty" json:"message,omitempty"`
	NextRunTime   time.Time `bson:"next_run_time,omitempty" json:"next_run_time,omitempty"`
}

type WorkstationConfig struct {
	SetupCommands []WorkstationSetupCommand `bson:"setup_commands" json:"setup_commands" yaml:"setup_commands"`
	GitClone      *bool                     `bson:"git_clone" json:"git_clone" yaml:"git_clone"`
}

type WorkstationSetupCommand struct {
	Command   string `bson:"command" json:"command" yaml:"command"`
	Directory string `bson:"directory" json:"directory" yaml:"directory"`
}

type GithubProjectConflicts struct {
	CommitQueueIdentifiers []string
	PRTestingIdentifiers   []string
	CommitCheckIdentifiers []string
}

type EmailAlertData struct {
	Recipients []string `bson:"recipients"`
}

var (
	// bson fields for the ProjectRef struct
	ProjectRefIdKey                                 = bsonutil.MustHaveTag(ProjectRef{}, "Id")
	ProjectRefOwnerKey                              = bsonutil.MustHaveTag(ProjectRef{}, "Owner")
	ProjectRefRepoKey                               = bsonutil.MustHaveTag(ProjectRef{}, "Repo")
	ProjectRefBranchKey                             = bsonutil.MustHaveTag(ProjectRef{}, "Branch")
	ProjectRefEnabledKey                            = bsonutil.MustHaveTag(ProjectRef{}, "Enabled")
	ProjectRefRestrictedKey                         = bsonutil.MustHaveTag(ProjectRef{}, "Restricted")
	ProjectRefBatchTimeKey                          = bsonutil.MustHaveTag(ProjectRef{}, "BatchTime")
	ProjectRefIdentifierKey                         = bsonutil.MustHaveTag(ProjectRef{}, "Identifier")
	ProjectRefRepoRefIdKey                          = bsonutil.MustHaveTag(ProjectRef{}, "RepoRefId")
	ProjectRefDisplayNameKey                        = bsonutil.MustHaveTag(ProjectRef{}, "DisplayName")
	ProjectRefDeactivatePreviousKey                 = bsonutil.MustHaveTag(ProjectRef{}, "DeactivatePrevious")
	ProjectRefRemotePathKey                         = bsonutil.MustHaveTag(ProjectRef{}, "RemotePath")
	ProjectRefHiddenKey                             = bsonutil.MustHaveTag(ProjectRef{}, "Hidden")
	ProjectRefRepotrackerErrorKey                   = bsonutil.MustHaveTag(ProjectRef{}, "RepotrackerError")
	ProjectRefDisabledStatsCacheKey                 = bsonutil.MustHaveTag(ProjectRef{}, "DisabledStatsCache")
	ProjectRefAdminsKey                             = bsonutil.MustHaveTag(ProjectRef{}, "Admins")
	ProjectRefGitTagAuthorizedUsersKey              = bsonutil.MustHaveTag(ProjectRef{}, "GitTagAuthorizedUsers")
	ProjectRefGitTagAuthorizedTeamsKey              = bsonutil.MustHaveTag(ProjectRef{}, "GitTagAuthorizedTeams")
	ProjectRefTracksPushEventsKey                   = bsonutil.MustHaveTag(ProjectRef{}, "TracksPushEvents")
	projectRefPRTestingEnabledKey                   = bsonutil.MustHaveTag(ProjectRef{}, "PRTestingEnabled")
	projectRefManualPRTestingEnabledKey             = bsonutil.MustHaveTag(ProjectRef{}, "ManualPRTestingEnabled")
	projectRefGithubChecksEnabledKey                = bsonutil.MustHaveTag(ProjectRef{}, "GithubChecksEnabled")
	projectRefGitTagVersionsEnabledKey              = bsonutil.MustHaveTag(ProjectRef{}, "GitTagVersionsEnabled")
	projectRefRepotrackerDisabledKey                = bsonutil.MustHaveTag(ProjectRef{}, "RepotrackerDisabled")
	projectRefCommitQueueKey                        = bsonutil.MustHaveTag(ProjectRef{}, "CommitQueue")
	projectRefTaskSyncKey                           = bsonutil.MustHaveTag(ProjectRef{}, "TaskSync")
	projectRefPatchingDisabledKey                   = bsonutil.MustHaveTag(ProjectRef{}, "PatchingDisabled")
	projectRefDispatchingDisabledKey                = bsonutil.MustHaveTag(ProjectRef{}, "DispatchingDisabled")
	projectRefStepbackDisabledKey                   = bsonutil.MustHaveTag(ProjectRef{}, "StepbackDisabled")
	projectRefStepbackBisectKey                     = bsonutil.MustHaveTag(ProjectRef{}, "StepbackBisect")
	projectRefVersionControlEnabledKey              = bsonutil.MustHaveTag(ProjectRef{}, "VersionControlEnabled")
	projectRefNotifyOnFailureKey                    = bsonutil.MustHaveTag(ProjectRef{}, "NotifyOnBuildFailure")
	projectRefSpawnHostScriptPathKey                = bsonutil.MustHaveTag(ProjectRef{}, "SpawnHostScriptPath")
	projectRefTriggersKey                           = bsonutil.MustHaveTag(ProjectRef{}, "Triggers")
	projectRefPatchTriggerAliasesKey                = bsonutil.MustHaveTag(ProjectRef{}, "PatchTriggerAliases")
	projectRefGithubTriggerAliasesKey               = bsonutil.MustHaveTag(ProjectRef{}, "GithubTriggerAliases")
	projectRefPeriodicBuildsKey                     = bsonutil.MustHaveTag(ProjectRef{}, "PeriodicBuilds")
	projectRefOldestAllowedMergeBaseKey             = bsonutil.MustHaveTag(ProjectRef{}, "OldestAllowedMergeBase")
	projectRefWorkstationConfigKey                  = bsonutil.MustHaveTag(ProjectRef{}, "WorkstationConfig")
	projectRefTaskAnnotationSettingsKey             = bsonutil.MustHaveTag(ProjectRef{}, "TaskAnnotationSettings")
	projectRefBuildBaronSettingsKey                 = bsonutil.MustHaveTag(ProjectRef{}, "BuildBaronSettings")
	projectRefPerfEnabledKey                        = bsonutil.MustHaveTag(ProjectRef{}, "PerfEnabled")
	projectRefContainerSecretsKey                   = bsonutil.MustHaveTag(ProjectRef{}, "ContainerSecrets")
	projectRefContainerSizeDefinitionsKey           = bsonutil.MustHaveTag(ProjectRef{}, "ContainerSizeDefinitions")
	projectRefExternalLinksKey                      = bsonutil.MustHaveTag(ProjectRef{}, "ExternalLinks")
	projectRefBannerKey                             = bsonutil.MustHaveTag(ProjectRef{}, "Banner")
	projectRefParsleyFiltersKey                     = bsonutil.MustHaveTag(ProjectRef{}, "ParsleyFilters")
	projectRefProjectHealthViewKey                  = bsonutil.MustHaveTag(ProjectRef{}, "ProjectHealthView")
	projectRefGitHubDynamicTokenPermissionGroupsKey = bsonutil.MustHaveTag(ProjectRef{}, "GitHubDynamicTokenPermissionGroups")
	projectRefGithubPermissionGroupByRequesterKey   = bsonutil.MustHaveTag(ProjectRef{}, "GitHubPermissionGroupByRequester")

	commitQueueEnabledKey          = bsonutil.MustHaveTag(CommitQueueParams{}, "Enabled")
	commitQueueMergeQueueKey       = bsonutil.MustHaveTag(CommitQueueParams{}, "MergeQueue")
	triggerDefinitionProjectKey    = bsonutil.MustHaveTag(TriggerDefinition{}, "Project")
	containerSecretExternalNameKey = bsonutil.MustHaveTag(ContainerSecret{}, "ExternalName")
	containerSecretExternalIDKey   = bsonutil.MustHaveTag(ContainerSecret{}, "ExternalID")
)

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
	return p.IsAutoPRTestingEnabled() || p.IsManualPRTestingEnabled()
}

func (p *ProjectRef) IsStepbackDisabled() bool {
	return utility.FromBoolPtr(p.StepbackDisabled)
}

func (p *ProjectRef) IsStepbackBisect() bool {
	return utility.FromBoolPtr(p.StepbackBisect)
}

func (p *ProjectRef) IsAutoPRTestingEnabled() bool {
	return utility.FromBoolPtr(p.PRTestingEnabled)
}

func (p *ProjectRef) IsManualPRTestingEnabled() bool {
	return utility.FromBoolPtr(p.ManualPRTestingEnabled)
}

func (p *ProjectRef) IsPRTestingEnabledByCaller(caller string) bool {
	switch caller {
	case patch.ManualCaller:
		return p.IsManualPRTestingEnabled()
	case patch.AutomatedCaller:
		return p.IsAutoPRTestingEnabled()
	default:
		return p.IsPRTestingEnabled()
	}
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

func (p *ProjectRef) IsGitTagVersionsEnabled() bool {
	return utility.FromBoolPtr(p.GitTagVersionsEnabled)
}

func (p *ProjectRef) IsStatsCacheDisabled() bool {
	return utility.FromBoolPtr(p.DisabledStatsCache)
}

func (p *ProjectRef) IsHidden() bool {
	return utility.FromBoolPtr(p.Hidden)
}

func (p *ProjectRef) UseRepoSettings() bool {
	return p.RepoRefId != ""
}

func (p *ProjectRef) DoesTrackPushEvents() bool {
	return utility.FromBoolPtr(p.TracksPushEvents)
}

func (p *ProjectRef) IsVersionControlEnabled() bool {
	return utility.FromBoolPtr(p.VersionControlEnabled)
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
	ProjectTriggerLevelPush  = "push"
	intervalPrefix           = "@every"
	maxBatchTime             = 153722867 // math.MaxInt64 / 60 / 1_000_000_000
)

type ProjectPageSection string

// These values must remain consistent with the GraphQL enum ProjectSettingsSection.
const (
	ProjectPageGeneralSection           = "GENERAL"
	ProjectPageAccessSection            = "ACCESS"
	ProjectPageVariablesSection         = "VARIABLES"
	ProjectPageNotificationsSection     = "NOTIFICATIONS"
	ProjectPagePatchAliasSection        = "PATCH_ALIASES"
	ProjectPageWorkstationsSection      = "WORKSTATION"
	ProjectPageTriggersSection          = "TRIGGERS"
	ProjectPagePeriodicBuildsSection    = "PERIODIC_BUILDS"
	ProjectPagePluginSection            = "PLUGINS"
	ProjectPageContainerSection         = "CONTAINERS"
	ProjectPageViewsAndFiltersSection   = "VIEWS_AND_FILTERS"
	ProjectPageGithubAndCQSection       = "GITHUB_AND_COMMIT_QUEUE"
	ProjectPageGithubAppSettingsSection = "GITHUB_APP_SETTINGS"
	ProjectPageGithubPermissionsSection = "GITHUB_PERMISSIONS"
)

const (
	tasksByProjectQueryMaxTime = 90 * time.Second
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
	// Ensure that any new project is originally explicitly disabled and set to private.
	p.Enabled = false

	// if a hidden project exists for this configuration, use that ID
	if p.Owner != "" && p.Repo != "" && p.Branch != "" {
		hidden, err := FindHiddenProjectRefByOwnerRepoAndBranch(p.Owner, p.Repo, p.Branch)
		if err != nil {
			return errors.Wrap(err, "finding hidden project")
		}
		if hidden != nil {
			p.Id = hidden.Id
			err := p.Upsert()
			if err != nil {
				return errors.Wrapf(err, "upserting project ref '%s'", hidden.Id)
			}
			if creator != nil {
				_, err = p.UpdateAdminRoles([]string{creator.Id}, nil)
				return err
			}
			return nil
		}
	}

	err := db.Insert(ProjectRefCollection, p)
	if err != nil {
		return errors.Wrap(err, "inserting project ref")
	}
	if err = commitqueue.EnsureCommitQueueExistsForProject(p.Id); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "error ensuring commit queue exists",
			"project_id":         p.Id,
			"project_identifier": p.Identifier,
		}))
	}

	newProjectVars := ProjectVars{
		Id: p.Id,
	}
	if err = newProjectVars.Insert(); err != nil {
		return errors.Wrapf(err, "adding project variables for project '%s'", p.Id)
	}
	return p.addPermissions(creator)
}

func (p *ProjectRef) GetPatchTriggerAlias(aliasName string) (patch.PatchTriggerDefinition, bool) {
	for _, alias := range p.PatchTriggerAliases {
		if alias.Alias == aliasName {
			return alias, true
		}
	}

	return patch.PatchTriggerDefinition{}, false
}

// MergeWithProjectConfig looks up the project config with the given project ref id and modifies
// the project ref scanning for any properties that can be set on both project ref and project parser.
// Any values that are set at the project config level will be set on the project ref IF they are not set on
// the project ref. If the version isn't specified, we get the latest config.
func (p *ProjectRef) MergeWithProjectConfig(version string) (err error) {
	projectConfig, err := FindProjectConfigForProjectOrVersion(p.Id, version)
	if err != nil {
		return err
	}
	if projectConfig != nil {
		defer func() {
			err = recovery.HandlePanicWithError(recover(), err, "project ref and project config structures do not match")
		}()
		pRefToMerge := ProjectRef{
			GithubTriggerAliases:     projectConfig.GithubTriggerAliases,
			ContainerSizeDefinitions: projectConfig.ContainerSizeDefinitions,
		}
		if projectConfig.WorkstationConfig != nil {
			pRefToMerge.WorkstationConfig = *projectConfig.WorkstationConfig
		}
		if projectConfig.BuildBaronSettings != nil {
			pRefToMerge.BuildBaronSettings = *projectConfig.BuildBaronSettings
		}
		if projectConfig.TaskAnnotationSettings != nil {
			pRefToMerge.TaskAnnotationSettings = *projectConfig.TaskAnnotationSettings
		}
		if projectConfig.TaskSync != nil {
			pRefToMerge.TaskSync = *projectConfig.TaskSync
		}
		reflectedRef := reflect.ValueOf(p).Elem()
		reflectedConfig := reflect.ValueOf(pRefToMerge)
		util.RecursivelySetUndefinedFields(reflectedRef, reflectedConfig)
	}
	return err
}

// SetGitHubAppCredentials updates or creates an entry in
// GithubAppAuth for the project ref. If the provided values
// are empty, the entry is deleted.
func (p *ProjectRef) SetGithubAppCredentials(appID int64, privateKey []byte) error {
	if appID == 0 && len(privateKey) == 0 {
		return githubapp.RemoveGithubAppAuth(p.Id)
	}

	if appID == 0 || len(privateKey) == 0 {
		return errors.New("both app ID and private key must be provided")
	}
	auth := githubapp.GithubAppAuth{
		Id:         p.Id,
		AppID:      appID,
		PrivateKey: privateKey,
	}
	return githubapp.UpsertGithubAppAuth(&auth)
}

// AddToRepoScope validates that the branch can be attached to the matching repo,
// adds the branch to the unrestricted branches under repo scope, and
// adds repo view permission for branch admins, and adds branch edit access for repo admins.
func (p *ProjectRef) AddToRepoScope(u *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	repoRef, err := FindRepoRefByOwnerAndRepo(p.Owner, p.Repo)
	if err != nil {
		return errors.Wrapf(err, "finding repo ref '%s'", p.RepoRefId)
	}
	if repoRef == nil {
		repoRef, err = p.createNewRepoRef(u)
		if err != nil {
			return errors.Wrapf(err, "creating new repo ref")
		}
	}
	if p.RepoRefId == "" {
		p.RepoRefId = repoRef.Id
	}

	// Add the project to the repo admin scope.
	if err := rm.AddResourceToScope(GetRepoAdminScope(p.RepoRefId), p.Id); err != nil {
		return errors.Wrapf(err, "adding resource to repo '%s' admin scope", p.RepoRefId)
	}
	// Only give branch admins view access if the repo isn't restricted.
	if !repoRef.IsRestricted() {
		if err := addViewRepoPermissionsToBranchAdmins(p.RepoRefId, p.Admins); err != nil {
			return errors.Wrapf(err, "giving branch '%s' admins view permission for repo '%s'", p.Id, p.RepoRefId)
		}
	}
	// If the branch is unrestricted, add it to this scope so users who requested all-repo permissions have access.
	if !p.IsRestricted() {
		if err := rm.AddResourceToScope(GetUnrestrictedBranchProjectsScope(p.RepoRefId), p.Id); err != nil {
			return errors.Wrap(err, "adding resource to unrestricted branches scope")
		}
	}
	return nil
}

// DetachFromRepo removes the branch from the relevant repo scopes, and updates the project to not point to the repo.
// Any values that previously defaulted to repo will have the repo value explicitly set.
func (p *ProjectRef) DetachFromRepo(u *user.DBUser) error {
	before, err := GetProjectSettingsById(p.Id, false)
	if err != nil {
		return errors.Wrap(err, "getting before project settings event")
	}

	// remove from relevant repo scopes
	if err = p.RemoveFromRepoScope(); err != nil {
		return err
	}

	mergedProject, err := FindMergedProjectRef(p.Id, "", false)
	if err != nil {
		return errors.Wrap(err, "finding merged project ref")
	}
	if mergedProject == nil {
		return errors.Errorf("project ref '%s' doesn't exist", p.Id)
	}

	// Save repo variables that don't exist in the repo as the project variables.
	// Wait to save merged project until we've gotten the variables.
	mergedVars, err := FindMergedProjectVars(before.ProjectRef.Id)
	if err != nil {
		return errors.Wrap(err, "finding merged project vars")
	}

	mergedProject.RepoRefId = ""
	if err := mergedProject.Upsert(); err != nil {
		return errors.Wrap(err, "detaching project from repo")
	}

	// catch any resulting errors so that we log before returning
	catcher := grip.NewBasicCatcher()
	if mergedVars != nil {
		_, err = mergedVars.Upsert()
		catcher.Wrap(err, "saving merged vars")
	}

	if len(before.Subscriptions) == 0 {
		// Save repo subscriptions as project subscriptions if none exist
		subs, err := event.FindSubscriptionsByOwner(before.ProjectRef.RepoRefId, event.OwnerTypeProject)
		catcher.Wrap(err, "finding repo subscriptions")

		for _, s := range subs {
			s.ID = ""
			s.Owner = p.Id
			catcher.Add(s.Upsert())
		}
	}

	// Handle each category of aliases as its own case
	repoAliases, err := FindAliasesForRepo(before.ProjectRef.RepoRefId)
	catcher.Wrap(err, "finding repo aliases")

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

	catcher.Add(GetAndLogProjectRepoAttachment(p.Id, u.Id, event.EventTypeProjectDetachedFromRepo, false, before))
	return catcher.Resolve()
}

// AttachToRepo adds the branch to the relevant repo scopes, and updates the project to point to the repo.
// Any values that previously were unset will now use the repo value, unless this would introduce
// a GitHub project conflict. If no repo ref currently exists, the user attaching it will be added as the repo ref admin.
func (p *ProjectRef) AttachToRepo(ctx context.Context, u *user.DBUser) error {
	// Before allowing a project to attach to a repo, verify that this is a valid GitHub organization.
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting config")
	}
	if err := p.ValidateOwnerAndRepo(config.GithubOrgs); err != nil {
		return errors.Wrap(err, "validating new owner/repo")
	}
	before, err := GetProjectSettingsById(p.Id, false)
	if err != nil {
		return errors.Wrap(err, "getting before project settings event")
	}
	if err := p.AddToRepoScope(u); err != nil {
		return err
	}
	update := bson.M{
		ProjectRefRepoRefIdKey: p.RepoRefId, // This is set locally in AddToRepoScope
	}
	update = p.addGithubConflictsToUpdate(update)
	err = db.UpdateId(ProjectRefCollection, p.Id, bson.M{
		"$set":   update,
		"$unset": bson.M{ProjectRefTracksPushEventsKey: 1},
	})
	if err != nil {
		return errors.Wrap(err, "attaching repo to scope")
	}

	return GetAndLogProjectRepoAttachment(p.Id, u.Id, event.EventTypeProjectAttachedToRepo, false, before)
}

// AttachToNewRepo modifies the project's owner/repo, updates the old and new repo scopes (if relevant), and
// updates the project to point to the new repo. Any Github project conflicts are disabled.
// If no repo ref currently exists for the new repo, the user attaching it will be added as the repo ref admin.
func (p *ProjectRef) AttachToNewRepo(u *user.DBUser) error {
	before, err := GetProjectSettingsById(p.Id, false)
	if err != nil {
		return errors.Wrap(err, "getting before project settings event")
	}

	allowedOrgs := evergreen.GetEnvironment().Settings().GithubOrgs
	if err := p.ValidateOwnerAndRepo(allowedOrgs); err != nil {
		return errors.Wrap(err, "validating new owner/repo")
	}

	if p.UseRepoSettings() {
		if err := p.RemoveFromRepoScope(); err != nil {
			return errors.Wrap(err, "removing project from old repo scope")
		}
		if err := p.AddToRepoScope(u); err != nil {
			return errors.Wrap(err, "adding project to new repo scope")
		}
	}

	update := bson.M{
		ProjectRefOwnerKey:     p.Owner,
		ProjectRefRepoKey:      p.Repo,
		ProjectRefRepoRefIdKey: p.RepoRefId,
	}
	update = p.addGithubConflictsToUpdate(update)

	err = db.UpdateId(ProjectRefCollection, p.Id, bson.M{
		"$set":   update,
		"$unset": bson.M{ProjectRefTracksPushEventsKey: 1},
	})
	if err != nil {
		return errors.Wrap(err, "updating owner/repo in the DB")
	}

	return GetAndLogProjectRepoAttachment(p.Id, u.Id, event.EventTypeProjectAttachedToRepo, false, before)
}

// addGithubConflictsToUpdate turns off any settings that may introduce conflicts by
// adding fields to the given update and returning them.
func (p *ProjectRef) addGithubConflictsToUpdate(update bson.M) bson.M {
	// If the project ref doesn't default to repo, will just return the original project.
	mergedProject, err := GetProjectRefMergedWithRepo(*p)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message":            "unable to merge project with attached repo",
			"project_id":         p.Id,
			"project_identifier": p.Identifier,
			"repo_id":            p.RepoRefId,
		}))
		return update
	}
	if mergedProject.Enabled {
		conflicts, err := mergedProject.GetGithubProjectConflicts()
		if err != nil {
			grip.Debug(message.WrapError(err, message.Fields{
				"message":            "unable to get github project conflicts",
				"project_id":         p.Id,
				"project_identifier": p.Identifier,
				"repo_id":            mergedProject.RepoRefId,
			}))
			return update
		}
		if len(conflicts.CommitQueueIdentifiers) > 0 {
			update[bsonutil.GetDottedKeyName(projectRefCommitQueueKey, commitQueueEnabledKey)] = false
		}
		if len(conflicts.CommitCheckIdentifiers) > 0 {
			update[projectRefGithubChecksEnabledKey] = false
		}
		if len(conflicts.PRTestingIdentifiers) > 0 {
			update[projectRefPRTestingEnabledKey] = false
		}
	}
	return update
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
			return errors.Wrap(err, "removing resource from unrestricted branches scope")
		}
	}
	if err := removeViewRepoPermissionsFromBranchAdmins(p.RepoRefId, p.Admins); err != nil {
		return errors.Wrap(err, "removing view repo permissions from branch admins")
	}
	if err := rm.RemoveResourceFromScope(GetRepoAdminScope(p.RepoRefId), p.Id); err != nil {
		return errors.Wrapf(err, "removing admin scope from repo '%s'", p.Repo)
	}
	p.RepoRefId = ""
	return nil
}

// addPermissions adds the project ref to the general scope (and repo scope if applicable) and
// gives the inputted creator admin permissions.
func (p *ProjectRef) addPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	parentScope := evergreen.UnrestrictedProjectsScope
	if p.IsRestricted() {
		parentScope = evergreen.RestrictedProjectsScope
	}
	if err := rm.AddResourceToScope(parentScope, p.Id); err != nil {
		return errors.Wrapf(err, "adding project '%s' to the scope '%s'", p.Id, parentScope)
	}

	// add scope for the branch-level project configurations
	newScope := gimlet.Scope{
		ID:        fmt.Sprintf("project_%s", p.Id),
		Resources: []string{p.Id},
		Name:      p.Id,
		Type:      evergreen.ProjectResourceType,
	}
	if err := rm.AddScope(newScope); err != nil {
		return errors.Wrapf(err, "adding scope for project '%s'", p.Id)
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
		return errors.Wrapf(err, "adding admin role for project '%s'", p.Id)
	}
	if creator != nil {
		if err := creator.AddRole(newRole.ID); err != nil {
			return errors.Wrapf(err, "adding role '%s' to user '%s'", newRole.ID, creator.Id)
		}
	}
	if p.UseRepoSettings() {
		if err := p.AddToRepoScope(creator); err != nil {
			return errors.Wrapf(err, "adding project to repo '%s'", p.RepoRefId)
		}
	}
	return nil
}

func findOneProjectRefQ(query db.Q) (*ProjectRef, error) {
	projectRef := &ProjectRef{}
	err := db.FindOneQ(ProjectRefCollection, query, projectRef)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return projectRef, err

}

// FindBranchProjectRef gets a project ref given the project identifier.
// This returns only branch-level settings; to include repo settings, use FindMergedProjectRef.
func FindBranchProjectRef(identifier string) (*ProjectRef, error) {
	return findOneProjectRefQ(byId(identifier))
}

// FindMergedProjectRef also finds the repo ref settings and merges relevant fields.
// Relevant fields will also be merged from the parser project with a specified version.
// If no version is specified, the most recent valid parser project version will be used for merge.
func FindMergedProjectRef(identifier string, version string, includeProjectConfig bool) (*ProjectRef, error) {
	pRef, err := FindBranchProjectRef(identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref '%s'", identifier)
	}
	if pRef == nil {
		return nil, nil
	}
	if pRef.UseRepoSettings() {
		repoRef, err := FindOneRepoRef(pRef.RepoRefId)
		if err != nil {
			return nil, errors.Wrapf(err, "finding repo ref '%s' for project '%s'", pRef.RepoRefId, pRef.Identifier)
		}
		if repoRef == nil {
			return nil, errors.Errorf("repo ref '%s' does not exist for project '%s'", pRef.RepoRefId, pRef.Identifier)
		}
		pRef, err = mergeBranchAndRepoSettings(pRef, repoRef)
		if err != nil {
			return nil, errors.Wrapf(err, "merging repo ref '%s' for project '%s'", repoRef.RepoRefId, identifier)
		}
	}
	if includeProjectConfig && pRef.IsVersionControlEnabled() {
		err = pRef.MergeWithProjectConfig(version)
		if err != nil {
			return nil, errors.Wrapf(err, "merging project config with project ref '%s'", pRef.Identifier)
		}
	}
	return pRef, nil
}

// GetNumberOfEnabledProjects returns the current number of enabled projects on evergreen.
func GetNumberOfEnabledProjects() (int, error) {
	// Empty owner and repo will return all enabled project count.
	return getNumberOfEnabledProjects("", "")
}

// GetNumberOfEnabledProjectsForOwnerRepo returns the number of enabled projects for a given owner/repo.
func GetNumberOfEnabledProjectsForOwnerRepo(owner, repo string) (int, error) {
	if owner == "" || repo == "" {
		return 0, errors.New("owner and repo must be specified")
	}
	return getNumberOfEnabledProjects(owner, repo)
}

func getNumberOfEnabledProjects(owner, repo string) (int, error) {
	pipeline := []bson.M{
		{"$match": bson.M{ProjectRefEnabledKey: true}},
	}
	if owner != "" && repo != "" {
		// Check owner and repos in project ref or repo ref.
		pipeline = append(pipeline, bson.M{"$match": byOwnerAndRepo(owner, repo)})
	}
	pipeline = append(pipeline, bson.M{"$count": "count"})
	type Count struct {
		Count int `bson:"count"`
	}
	count := []Count{}
	err := db.Aggregate(ProjectRefCollection, pipeline, &count)
	if err != nil {
		return 0, err
	}
	if len(count) == 0 {
		return 0, nil
	}
	return count[0].Count, nil
}

// GetProjectRefMergedWithRepo merges the project with the repo, if one exists.
// Otherwise, it will return the project as given.
func GetProjectRefMergedWithRepo(pRef ProjectRef) (*ProjectRef, error) {
	if !pRef.UseRepoSettings() {
		return &pRef, nil
	}
	repoRef, err := FindOneRepoRef(pRef.RepoRefId)
	if err != nil {
		return nil, errors.Wrapf(err, "finding repo ref '%s'", pRef.RepoRefId)
	}
	if repoRef == nil {
		return nil, errors.Errorf("repo ref '%s' does not exist", pRef.RepoRefId)
	}
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

	// Include Parsley filters defined at repo level alongside project filters.
	mergeParsleyFilters(pRef, repoRef)

	util.RecursivelySetUndefinedFields(reflectedBranch, reflectedRepo)

	return pRef, err
}

func mergeParsleyFilters(pRef *ProjectRef, repoRef *RepoRef) {
	if len(repoRef.ParsleyFilters) == 0 {
		return
	}

	if pRef.ParsleyFilters == nil {
		pRef.ParsleyFilters = []parsley.Filter{}
	}

	pRef.ParsleyFilters = append(pRef.ParsleyFilters, repoRef.ParsleyFilters...)
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
		Admins: []string{},
	}}

	allEnabledProjects, err := FindMergedEnabledProjectRefsByOwnerAndRepo(p.Owner, p.Repo)
	if err != nil {
		return nil, errors.Wrap(err, "finding all enabled projects")
	}
	// For every setting in the project ref, if all enabled projects have the same setting, then use that.
	defer func() {
		err = recovery.HandlePanicWithError(recover(), err, "project and repo structures do not match")
	}()
	setRepoFieldsFromProjects(repoRef, allEnabledProjects)
	if !utility.StringSliceContains(repoRef.Admins, u.Username()) {
		repoRef.Admins = append(repoRef.Admins, u.Username())
	}
	// Some fields shouldn't be set from projects.
	repoRef.Id = mgobson.NewObjectId().Hex()
	repoRef.RepoRefId = ""
	repoRef.Identifier = ""

	// Set explicitly in case no project is enabled.
	repoRef.Owner = p.Owner
	repoRef.Repo = p.Repo
	_, err = SetTracksPushEvents(context.Background(), &repoRef.ProjectRef)
	if err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "error setting project tracks push events",
			"repo_id": repoRef.Id,
			"owner":   repoRef.Owner,
			"repo":    repoRef.Repo,
		}))
	}

	// Creates scope and give user admin access to repo.
	if err = repoRef.Add(u); err != nil {
		return nil, errors.Wrapf(err, "adding new repo repo ref for '%s/%s'", p.Owner, p.Repo)
	}
	err = LogProjectAdded(repoRef.Id, u.DisplayName())
	grip.Error(message.WrapError(err, message.Fields{
		"message":            "problem logging repo added",
		"project_id":         repoRef.Id,
		"project_identifier": repoRef.Identifier,
		"user":               u.DisplayName(),
	}))

	enabledProjectIds := []string{}
	for _, p := range allEnabledProjects {
		enabledProjectIds = append(enabledProjectIds, p.Id)
	}
	commonProjectVars, err := getCommonProjectVariables(enabledProjectIds)
	if err != nil {
		return nil, errors.Wrap(err, "getting common project variables")
	}
	commonProjectVars.Id = repoRef.Id
	if err = commonProjectVars.Insert(); err != nil {
		return nil, errors.Wrap(err, "inserting project variables for repo")
	}

	commonAliases, err := getCommonAliases(enabledProjectIds)
	if err != nil {
		return nil, errors.Wrap(err, "getting common project aliases")
	}
	for _, a := range commonAliases {
		a.ProjectID = repoRef.Id
		if err = a.Upsert(); err != nil {
			return nil, errors.Wrap(err, "upserting alias for repo")
		}
	}

	return repoRef, nil
}

func getCommonAliases(projectIds []string) (ProjectAliases, error) {
	commonAliases := []ProjectAlias{}
	for i, id := range projectIds {
		aliases, err := FindAliasesForProjectFromDb(id)
		if err != nil {
			return nil, errors.Wrap(err, "finding aliases for project")
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
	commonAdminOnly := map[string]bool{}
	for i, id := range projectIds {
		vars, err := FindOneProjectVars(id)
		if err != nil {
			return nil, errors.Wrapf(err, "finding variables for project '%s'", id)
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
			if vars.AdminOnlyVars != nil {
				commonAdminOnly = vars.AdminOnlyVars
			}
			continue
		}
		for key, val := range commonProjectVariables {
			// If the key is private/admin only in any of the projects, make it private/admin only in the repo.
			if vars.Vars[key] == val {
				if vars.PrivateVars[key] {
					commonPrivate[key] = true
				}
				if vars.AdminOnlyVars[key] {
					commonAdminOnly[key] = true
				}
			} else {
				// remove any variables from the common set that aren't in all the project refs
				delete(commonProjectVariables, key)
			}
		}
	}
	return &ProjectVars{
		Vars:          commonProjectVariables,
		PrivateVars:   commonPrivate,
		AdminOnlyVars: commonAdminOnly,
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

type GetProjectTasksOpts struct {
	Limit        int      `json:"num_versions"`
	BuildVariant string   `json:"build_variant"`
	StartAt      int      `json:"start_at"`
	Requesters   []string `json:"requesters"`
}

// GetTasksWithOptions will find the matching tasks run in the last number of versions(denoted by Limit) that exist for a given project.
// This function may also filter on tasks running on a specific build variant, or tasks that come after a specific revision order number.
func GetTasksWithOptions(projectName string, taskName string, opts GetProjectTasksOpts) ([]task.Task, error) {
	projectId, err := GetIdForProject(projectName)
	if err != nil {
		return nil, err
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultVersionLimit
	}
	finishedStatuses := append(evergreen.TaskFailureStatuses, evergreen.TaskSucceeded)
	match := bson.M{
		task.ProjectKey:     projectId,
		task.DisplayNameKey: taskName,
		task.StatusKey:      bson.M{"$in": finishedStatuses},
	}
	if len(opts.Requesters) > 0 {
		match[task.RequesterKey] = bson.M{"$in": opts.Requesters}
	} else {
		match[task.RequesterKey] = bson.M{"$in": evergreen.SystemVersionRequesterTypes}
	}
	if opts.BuildVariant != "" {
		match[task.BuildVariantKey] = opts.BuildVariant
	}
	startingRevision := opts.StartAt
	if startingRevision == 0 {
		repo, err := FindRepository(projectId)
		if err != nil {
			return nil, err
		}
		if repo == nil {
			return nil, errors.Errorf("finding repository '%s'", projectId)
		}
		startingRevision = repo.RevisionOrderNumber
	}
	match["$and"] = []bson.M{
		{task.RevisionOrderNumberKey: bson.M{"$lte": startingRevision}},
		{task.RevisionOrderNumberKey: bson.M{"$gte": startingRevision - opts.Limit + 1}},
	}
	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{task.RevisionOrderNumberKey: -1}})

	res := []task.Task{}
	if err = db.AggregateWithMaxTime(task.Collection, pipeline, &res, tasksByProjectQueryMaxTime); err != nil {
		return nil, errors.Wrapf(err, "aggregating tasks")
	}
	return res, nil
}

// FindAnyRestrictedProjectRef returns an unrestricted project to use as a default for contexts.
// TODO: Investigate removing this in DEVPROD-10469.
func FindAnyRestrictedProjectRef() (*ProjectRef, error) {
	projectRefs, err := FindAllMergedEnabledTrackedProjectRefs()
	if err != nil {
		return nil, errors.Wrap(err, "finding all project refs")
	}

	for _, pRef := range projectRefs {
		if pRef.IsRestricted() {
			continue
		}
		return &pRef, nil
	}
	return nil, errors.New("no projects available")
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

// FindAllMergedEnabledTrackedProjectRefs returns all enabled project refs in the db
// that are currently being tracked (i.e. their project files
// still exist and the project is not hidden).
func FindAllMergedEnabledTrackedProjectRefs() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	q := db.Query(bson.M{
		ProjectRefHiddenKey:  bson.M{"$ne": true},
		ProjectRefEnabledKey: true,
	})
	err := db.FindAllQ(ProjectRefCollection, q, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

func addLoggerAndRepoSettingsToProjects(pRefs []ProjectRef) ([]ProjectRef, error) {
	repoRefs := map[string]*RepoRef{} // cache repoRefs by id
	for i, pRef := range pRefs {
		if pRefs[i].UseRepoSettings() {
			repoRef := repoRefs[pRef.RepoRefId]
			if repoRef == nil {
				var err error
				repoRef, err = FindOneRepoRef(pRef.RepoRefId)
				if err != nil {
					return nil, errors.Wrapf(err, "finding repo ref '%s' for project '%s'", pRef.RepoRefId, pRef.Identifier)
				}
				if repoRef == nil {
					return nil, errors.Errorf("repo ref '%s' does not exist for project '%s'", pRef.RepoRefId, pRef.Identifier)
				}
				repoRefs[pRef.RepoRefId] = repoRef
			}
			mergedProject, err := mergeBranchAndRepoSettings(&pRefs[i], repoRef)
			if err != nil {
				return nil, errors.Wrap(err, "merging settings")
			}
			pRefs[i] = *mergedProject
		}
	}
	return pRefs, nil
}

// FindAllMergedProjectRefs returns all project refs in the db, with repo ref information merged
func FindAllMergedProjectRefs() ([]ProjectRef, error) {
	return findProjectRefsQ(bson.M{}, true)
}

func FindMergedProjectRefsByIds(ids ...string) ([]ProjectRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return findProjectRefsQ(bson.M{
		ProjectRefIdKey: bson.M{
			"$in": ids,
		},
	}, true)
}

// FindMergedEnabledProjectRefsByIds returns all project refs for the provided ids
// that are currently enabled.
func FindMergedEnabledProjectRefsByIds(ids ...string) ([]ProjectRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return findProjectRefsQ(bson.M{
		ProjectRefIdKey: bson.M{
			"$in": ids,
		},
		ProjectRefEnabledKey: true,
	}, true)
}

func FindProjectRefsByIds(ids ...string) ([]ProjectRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return findProjectRefsQ(bson.M{
		ProjectRefIdKey: bson.M{
			"$in": ids,
		},
	}, false)
}

func findProjectRefsQ(filter bson.M, merged bool) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	q := db.Query(filter)
	err := db.FindAllQ(ProjectRefCollection, q, &projectRefs)
	if err != nil {
		return nil, err
	}

	if merged {
		return addLoggerAndRepoSettingsToProjects(projectRefs)
	}
	return projectRefs, nil
}

func byOwnerAndRepo(owner, repoName string) bson.M {
	return bson.M{
		ProjectRefOwnerKey: owner,
		ProjectRefRepoKey:  repoName,
	}
}

// byOwnerRepoAndBranch excepts an owner, repoName, and branch.
// If includeUndefinedBranches is set, also returns projects with an empty branch, so this can
// be populated by the repo elsewhere.
func byOwnerRepoAndBranch(owner, repoName, branch string, includeUndefinedBranches bool) bson.M {
	q := bson.M{
		ProjectRefOwnerKey: owner,
		ProjectRefRepoKey:  repoName,
	}
	if includeUndefinedBranches {
		q["$or"] = []bson.M{
			{ProjectRefBranchKey: ""},
			{ProjectRefBranchKey: branch},
		}
	} else {
		q[ProjectRefBranchKey] = branch
	}
	return q
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

	match := byOwnerRepoAndBranch(owner, repoName, branch, true)
	match[ProjectRefEnabledKey] = true
	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, lookupRepoStep)
	err := db.Aggregate(ProjectRefCollection, pipeline, &projectRefs)
	if err != nil {
		return nil, err
	}
	mergedProjects, err := addLoggerAndRepoSettingsToProjects(projectRefs)
	if err != nil {
		return nil, err
	}
	return filterProjectsByBranch(mergedProjects, branch), nil
}

// FindMergedProjectRefsThatUseRepoSettingsByRepoAndBranch finds ProjectRef with matching repo/branch that
// rely on the repo configuration, and merges that info.
func FindMergedProjectRefsThatUseRepoSettingsByRepoAndBranch(owner, repoName, branch string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	q := byOwnerRepoAndBranch(owner, repoName, branch, true)
	q[ProjectRefRepoRefIdKey] = bson.M{"$exists": true, "$ne": ""}
	pipeline := []bson.M{{"$match": q}}
	err := db.Aggregate(ProjectRefCollection, pipeline, &projectRefs)
	if err != nil {
		return nil, err
	}
	mergedProjects, err := addLoggerAndRepoSettingsToProjects(projectRefs)
	if err != nil {
		return nil, err
	}
	return filterProjectsByBranch(mergedProjects, branch), nil
}

func filterProjectsByBranch(pRefs []ProjectRef, branch string) []ProjectRef {
	res := []ProjectRef{}
	for _, p := range pRefs {
		if p.Branch == branch {
			res = append(res, p)
		}
	}
	return res
}

func FindBranchAdminsForRepo(repoId string) ([]string, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAllQ(
		ProjectRefCollection,
		db.Query(bson.M{
			ProjectRefRepoRefIdKey: repoId,
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

// FindDownstreamProjects finds projects that have that trigger enabled or
// inherits it from the repo project.
func FindDownstreamProjects(project string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.Aggregate(ProjectRefCollection, projectRefPipelineForMatchingTrigger(project), &projectRefs)
	if err != nil {
		return nil, err
	}

	return projectRefs, err
}

// FindOneProjectRefByRepoAndBranchWithPRTesting finds a single ProjectRef with matching
// repo/branch that is enabled and setup for PR testing.
func FindOneProjectRefByRepoAndBranchWithPRTesting(owner, repo, branch, calledBy string) (*ProjectRef, error) {
	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching project ref for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}
	for _, p := range projectRefs {
		if p.IsPRTestingEnabledByCaller(calledBy) {
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
		return nil, errors.Wrapf(err, "finding merged repo refs for repo '%s/%s'", owner, repo)
	}
	if repoRef == nil || !repoRef.IsPRTestingEnabledByCaller(calledBy) {
		grip.Debug(message.Fields{
			"source":  "find project ref for PR testing",
			"message": "repo ref not configured for PR testing untracked branches",
			"owner":   owner,
			"repo":    repo,
			"branch":  branch,
		})
		return nil, nil
	}
	if repoRef.RemotePath == "" {
		grip.Error(message.Fields{
			"source":  "find project ref for PR testing",
			"message": "repo ref has no remote path, cannot use for PR testing",
			"owner":   owner,
			"repo":    repo,
			"branch":  branch,
		})
		return nil, errors.Errorf("repo ref '%s' has no remote path, cannot use for PR testing", repoRef.Id)
	}

	projectRefs, err = FindMergedProjectRefsThatUseRepoSettingsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "finding merged all project refs for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}

	// if a disabled project exists, then return early
	var hiddenProject *ProjectRef
	for i, p := range projectRefs {
		if !p.Enabled && !p.IsHidden() {
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
			Id:        mgobson.NewObjectId().Hex(),
			Owner:     owner,
			Repo:      repo,
			Branch:    branch,
			RepoRefId: repoRef.Id,
			Enabled:   false,
			Hidden:    utility.TruePtr(),
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

// FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch finds the project ref for this owner/repo/branch
// that has the commit queue enabled. There should only ever be one project for the query because we only enable commit
// queue if no other project ref with the same specification has it enabled.
func FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	projectRefs, err := FindMergedEnabledProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching project ref for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}
	for _, p := range projectRefs {
		if p.CommitQueue.IsEnabled() {
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

// SetTracksPushEvents returns true if the GitHub app is installed on the owner/repo for the given project.
func SetTracksPushEvents(ctx context.Context, projectRef *ProjectRef) (bool, error) {
	// Don't return errors because it could cause the project page to break if GitHub is down.
	hasApp, err := githubapp.CreateGitHubAppAuth(evergreen.GetEnvironment().Settings()).IsGithubAppInstalledOnRepo(ctx, projectRef.Owner, projectRef.Repo)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":            "Error verifying GitHub app installation",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"owner":              projectRef.Owner,
			"repo":               projectRef.Repo,
		}))
		projectRef.TracksPushEvents = utility.FalsePtr()
		return false, nil
	}
	// don't return error:
	// sometimes people change a project to track a personal
	// branch we don't have access to
	if !hasApp {
		grip.Warning(message.Fields{
			"message":            "GitHub app not installed",
			"project":            projectRef.Id,
			"project_identifier": projectRef.Identifier,
			"owner":              projectRef.Owner,
			"repo":               projectRef.Repo,
		})
		projectRef.TracksPushEvents = utility.FalsePtr()
		return false, nil
	}

	projectRef.TracksPushEvents = utility.TruePtr()
	return true, nil
}

func UpdateAdminRoles(project *ProjectRef, toAdd, toDelete []string) error {
	if project == nil {
		return errors.New("no project found")
	}
	_, err := project.UpdateAdminRoles(toAdd, toDelete)
	return err
}

// FindProjects queries the backing database for the specified projects
func FindProjects(key string, limit int, sortDir int) ([]ProjectRef, error) {
	projects, err := FindProjectRefs(key, limit, sortDir)
	if err != nil {
		return nil, errors.Wrapf(err, "fetching projects starting at project '%s'", key)
	}

	return projects, nil
}

// UpdateProjectRevision updates the given project's revision
func UpdateProjectRevision(projectID, revision string) error {
	if err := UpdateLastRevision(projectID, revision); err != nil {
		return errors.Wrapf(err, "updating revision for project '%s'", projectID)
	}

	return nil
}

func FindHiddenProjectRefByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	// don't need to include undefined branches here since hidden projects explicitly define them
	q := byOwnerRepoAndBranch(owner, repo, branch, false)
	q[ProjectRefHiddenKey] = true

	return findOneProjectRefQ(db.Query(q))
}

func FindMergedEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	match := byOwnerAndRepo(owner, repo)
	match[ProjectRefEnabledKey] = true
	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, lookupRepoStep)
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
		if projectRefs[i].UseRepoSettings() {
			mergedProject, err := mergeBranchAndRepoSettings(&projectRefs[i], repoRef)
			if err != nil {
				return nil, errors.Wrap(err, "merging settings")
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
			return nil, errors.Wrap(err, "finding repo ref")
		}
		if repoRef == nil {
			return nil, errors.Errorf("repo ref '%s' not found", projectId)
		}
		return GetProjectSettings(&repoRef.ProjectRef)
	}

	pRef, err = FindBranchProjectRef(projectId)
	if err != nil {
		return nil, errors.Wrap(err, "finding project ref")
	}
	if pRef == nil {
		return nil, errors.Errorf("project ref '%s' not found", projectId)
	}

	return GetProjectSettings(pRef)
}

// GetProjectSettings returns the ProjectSettings of the given identifier and ProjectRef
func GetProjectSettings(p *ProjectRef) (*ProjectSettings, error) {
	// Don't error even if there is problem with verifying the GitHub app installation
	// because a GitHub outage could cause project settings page to not load.
	hasEvergreenAppInstalled, _ := githubapp.CreateGitHubAppAuth(evergreen.GetEnvironment().Settings()).IsGithubAppInstalledOnRepo(context.Background(), p.Owner, p.Repo)

	projectVars, err := FindOneProjectVars(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding variables for project '%s'", p.Id)
	}
	if projectVars == nil {
		projectVars = &ProjectVars{}
	}
	projectAliases, err := FindAliasesForProjectFromDb(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding aliases for project '%s'", p.Id)
	}
	subscriptions, err := event.FindSubscriptionsByOwner(p.Id, event.OwnerTypeProject)
	if err != nil {
		return nil, errors.Wrapf(err, "finding subscription for project '%s'", p.Id)
	}

	githubApp, err := githubapp.FindOneGithubAppAuth(p.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding GitHub app for project '%s'", p.Id)
	}
	if githubApp == nil {
		githubApp = &githubapp.GithubAppAuth{}
	}

	projectSettingsEvent := ProjectSettings{
		ProjectRef:         *p,
		GitHubAppAuth:      *githubApp,
		GithubHooksEnabled: hasEvergreenAppInstalled,
		Vars:               *projectVars,
		Aliases:            projectAliases,
		Subscriptions:      subscriptions,
	}

	return &projectSettingsEvent, nil
}

func IsPerfEnabledForProject(projectId string) bool {
	projectRef, err := FindMergedProjectRef(projectId, "", true)
	if err != nil || projectRef == nil {
		return false
	}
	return projectRef.IsPerfEnabled()
}

func UpdateOwnerAndRepoForBranchProjects(repoId, owner, repo string) error {
	return db.Update(
		ProjectRefCollection,
		bson.M{
			ProjectRefRepoRefIdKey: repoId,
		},
		bson.M{
			"$set": bson.M{
				ProjectRefOwnerKey: owner,
				ProjectRefRepoKey:  repo,
			},
		})
}

// FindProjectRefIdsWithCommitQueueEnabled returns a list of project IDs that have the commit queue enabled.
// We don't return the full projects since they aren't actually merged with the repo documents, so they
// aren't necessarily accurate.
func FindProjectRefIdsWithCommitQueueEnabled() ([]string, error) {
	projectRefs := []ProjectRef{}
	res := []string{}
	err := db.Aggregate(
		ProjectRefCollection,
		projectRefPipelineForCommitQueueEnabled(),
		&projectRefs)
	if err != nil {
		return nil, err
	}
	for _, p := range projectRefs {
		res = append(res, p.Id)
	}

	return res, nil
}

// FindPeriodicProjects returns a list of merged projects that have periodic builds defined.
func FindPeriodicProjects() ([]ProjectRef, error) {
	res := []ProjectRef{}

	projectRefs, err := FindAllMergedTrackedProjectRefs()
	if err != nil {
		return nil, err
	}
	for _, p := range projectRefs {
		if p.Enabled && len(p.PeriodicBuilds) > 0 {
			res = append(res, p)
		}
	}

	return res, nil
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

	return projectRefs, err
}

// ValidateEnabledRepotracker checks if the repotracker is being enabled,
// and if it is, checks to make sure it can be enabled.
func (p *ProjectRef) ValidateEnabledRepotracker() error {
	if !p.IsRepotrackerDisabled() && p.Enabled && p.RemotePath == "" {
		return errors.Errorf("remote path can't be empty for enabled repotracker project '%s'", p.Identifier)
	}
	return nil
}

func (p *ProjectRef) CanEnableCommitQueue() (bool, error) {
	conflicts, err := p.GetGithubProjectConflicts()
	if err != nil {
		return false, errors.Wrap(err, "finding GitHub conflicts")
	}
	if len(conflicts.CommitQueueIdentifiers) > 0 {
		return false, nil
	}
	return true, nil
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, a new one is created.
func (p *ProjectRef) Upsert() error {
	_, err := db.Upsert(ProjectRefCollection, bson.M{ProjectRefIdKey: p.Id}, p)
	return err
}

// SetRepotrackerError updates the repotracker error for the project ref.
func (p *ProjectRef) SetRepotrackerError(d *RepositoryErrorDetails) error {
	if err := db.UpdateId(ProjectRefCollection, p.Id, bson.M{
		"$set": bson.M{
			ProjectRefRepotrackerErrorKey: d,
		},
	}); err != nil {
		return err
	}
	p.RepotrackerError = d
	return nil
}

// SetContainerSecrets updates the container secrets for the project ref.
func (p *ProjectRef) SetContainerSecrets(secrets []ContainerSecret) error {
	if err := db.UpdateId(ProjectRefCollection, p.Id, bson.M{
		"$set": bson.M{
			projectRefContainerSecretsKey: secrets,
		},
	}); err != nil {
		return err
	}
	p.ContainerSecrets = secrets
	return nil
}

// SaveProjectPageForSection updates the project or repo ref variables for the section (if no project is given, we unset to default to repo).
func SaveProjectPageForSection(projectId string, p *ProjectRef, section ProjectPageSection, isRepo bool) (bool, error) {
	coll := ProjectRefCollection
	if isRepo {
		coll = RepoRefCollection
		if p == nil {
			return false, errors.New("can't default project ref for a repo")
		}
	}
	defaultToRepo := false
	if p == nil {
		defaultToRepo = true
		p = &ProjectRef{} // use a blank project ref to default the section to repo
	}

	var err error
	switch section {
	case ProjectPageGeneralSection:
		setUpdate := bson.M{
			ProjectRefBranchKey:                p.Branch,
			ProjectRefBatchTimeKey:             p.BatchTime,
			ProjectRefRemotePathKey:            p.RemotePath,
			projectRefSpawnHostScriptPathKey:   p.SpawnHostScriptPath,
			projectRefDispatchingDisabledKey:   p.DispatchingDisabled,
			projectRefStepbackDisabledKey:      p.StepbackDisabled,
			projectRefStepbackBisectKey:        p.StepbackBisect,
			projectRefVersionControlEnabledKey: p.VersionControlEnabled,
			ProjectRefDeactivatePreviousKey:    p.DeactivatePrevious,
			projectRefRepotrackerDisabledKey:   p.RepotrackerDisabled,
			projectRefPatchingDisabledKey:      p.PatchingDisabled,
			projectRefTaskSyncKey:              p.TaskSync,
			ProjectRefDisabledStatsCacheKey:    p.DisabledStatsCache,
		}
		// Unlike other fields, this will only be set if we're actually modifying it since it's used by the backend.
		if p.TracksPushEvents != nil {
			setUpdate[ProjectRefTracksPushEventsKey] = p.TracksPushEvents
		}
		// Allow a user to modify owner and repo only if they are editing an unattached project
		if !isRepo && !p.UseRepoSettings() && !defaultToRepo {
			setUpdate[ProjectRefOwnerKey] = p.Owner
			setUpdate[ProjectRefRepoKey] = p.Repo

		}
		// some fields shouldn't be set to nil when defaulting to the repo
		if !defaultToRepo {
			setUpdate[ProjectRefBranchKey] = p.Branch
			setUpdate[ProjectRefEnabledKey] = p.Enabled
			setUpdate[ProjectRefDisplayNameKey] = p.DisplayName
			setUpdate[ProjectRefIdentifierKey] = p.Identifier
		}
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": setUpdate,
			})
	case ProjectPagePluginSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefTaskAnnotationSettingsKey: p.TaskAnnotationSettings,
					projectRefBuildBaronSettingsKey:     p.BuildBaronSettings,
					projectRefPerfEnabledKey:            p.PerfEnabled,
					projectRefExternalLinksKey:          p.ExternalLinks,
				},
			})
	case ProjectPageAccessSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					ProjectRefRestrictedKey: p.Restricted,
					ProjectRefAdminsKey:     p.Admins,
				},
			})
	case ProjectPageGithubAndCQSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefPRTestingEnabledKey:       p.PRTestingEnabled,
					projectRefManualPRTestingEnabledKey: p.ManualPRTestingEnabled,
					projectRefGithubChecksEnabledKey:    p.GithubChecksEnabled,
					projectRefGitTagVersionsEnabledKey:  p.GitTagVersionsEnabled,
					ProjectRefGitTagAuthorizedUsersKey:  p.GitTagAuthorizedUsers,
					ProjectRefGitTagAuthorizedTeamsKey:  p.GitTagAuthorizedTeams,
					projectRefCommitQueueKey:            p.CommitQueue,
					projectRefOldestAllowedMergeBaseKey: p.OldestAllowedMergeBase,
				},
			})
	case ProjectPageNotificationsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{projectRefNotifyOnFailureKey: p.NotifyOnBuildFailure,
					projectRefBannerKey: p.Banner},
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
	case ProjectPagePatchAliasSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefPatchTriggerAliasesKey:  p.PatchTriggerAliases,
					projectRefGithubTriggerAliasesKey: p.GithubTriggerAliases,
				},
			})
	case ProjectPagePeriodicBuildsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{projectRefPeriodicBuildsKey: p.PeriodicBuilds},
			})
	case ProjectPageContainerSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{projectRefContainerSizeDefinitionsKey: p.ContainerSizeDefinitions},
			})
	case ProjectPageViewsAndFiltersSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefParsleyFiltersKey:    p.ParsleyFilters,
					projectRefProjectHealthViewKey: p.ProjectHealthView,
				},
			})
	case ProjectPageGithubAppSettingsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefGithubPermissionGroupByRequesterKey: p.GitHubPermissionGroupByRequester,
				},
			})
	case ProjectPageGithubPermissionsSection:
		err = db.Update(coll,
			bson.M{ProjectRefIdKey: projectId},
			bson.M{
				"$set": bson.M{
					projectRefGitHubDynamicTokenPermissionGroupsKey: p.GitHubDynamicTokenPermissionGroups,
				},
			})
	case ProjectPageVariablesSection:
		// this section doesn't modify the project/repo ref
		return false, nil
	default:
		return false, errors.Errorf("invalid section")
	}

	if err != nil {
		return false, errors.Wrap(err, "saving section")
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
		return errors.Wrap(err, "getting before project settings event")
	}

	modified, err := SaveProjectPageForSection(projectId, nil, section, false)
	if err != nil {
		return errors.Wrapf(err, "defaulting project ref to repo for section '%s'", section)
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
					projectVarsMapKey:   1,
					privateVarsMapKey:   1,
					adminOnlyVarsMapKey: 1,
				},
			})
		if err == nil {
			modified = true
		}
		catcher.Wrapf(err, "defaulting to repo for section '%s'", section)
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

	return errors.Wrapf(catcher.Resolve(), "defaulting to repo for section '%s'", section)
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

// GetNextCronTime returns the next valid batch time
func GetNextCronTime(baseTime time.Time, cronBatchTime string) (time.Time, error) {
	sched, err := getCronParserSchedule(cronBatchTime)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(baseTime), nil
}

func getCronParserSchedule(cronStr string) (cron.Schedule, error) {
	if strings.HasPrefix(cronStr, intervalPrefix) {
		return nil, errors.Errorf("cannot use interval '%s' in cron '%s'", intervalPrefix, cronStr)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
	sched, err := parser.Parse(cronStr)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing cron '%s'", cronStr)
	}
	return sched, nil
}

// GetActivationTimeForVariant returns the time at which this variant should
// next be activated. The version create time is used to determine the next
// activation time, except in situations where using the version create time
// would produce conflicts such as duplicate cron runs.
func (p *ProjectRef) GetActivationTimeForVariant(variant *BuildVariant, versionCreateTime time.Time, now time.Time) (time.Time, error) {
	// if we don't want to activate the build, set batchtime to the zero time
	if !utility.FromBoolTPtr(variant.Activate) {
		return utility.ZeroTime, nil
	}
	if variant.CronBatchTime != "" {
		// Prefer to schedule the cron activation time based on version create
		// time.
		proposedCron, err := GetNextCronTime(versionCreateTime, variant.CronBatchTime)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "getting next cron time")
		}
		isValidCron, err := p.isValidBVCron(variant, proposedCron, now)
		if err != nil {
			return time.Time{}, errors.Wrapf(err, "checking if proposed cron %s for variant '%s' is valid", proposedCron.String(), variant.Name)
		}
		if isValidCron {
			return proposedCron, nil
		}

		// If the cron is scheduled too far in the past or will conflict with
		// another cron activation, skip it and run the cron in the future
		// instead.
		return GetNextCronTime(now, variant.CronBatchTime)
	}
	// if activated explicitly set to true and we don't have batchtime, then we want to just activate now
	if utility.FromBoolPtr(variant.Activate) && variant.BatchTime == nil {
		return now, nil
	}

	lastActivated, err := VersionFindOne(VersionByLastVariantActivation(p.Id, variant.Name).WithFields(VersionBuildVariantsKey))
	if err != nil {
		return time.Time{}, errors.Wrap(err, "finding version")
	}

	if lastActivated == nil {
		return versionCreateTime, nil
	}

	// find matching activated build variant
	for _, buildStatus := range lastActivated.BuildVariants {
		if buildStatus.BuildVariant != variant.Name || !buildStatus.Activated {
			continue
		}
		if !buildStatus.ActivateAt.IsZero() {
			return buildStatus.ActivateAt.Add(time.Minute * time.Duration(p.getBatchTimeForVariant(variant))), nil
		}
	}

	return versionCreateTime, nil
}

// isActiveCronTimeRange checks that the proposed cron should activate now or
// has already activated very recently.
func (p *ProjectRef) isActiveCronTimeRange(proposedCron time.Time, now time.Time) bool {
	const cronActiveRange = 5 * time.Minute
	return !proposedCron.Before(now.Add(-cronActiveRange))
}

// isValidBVCron checks is a build variant cron is valid.
func (p *ProjectRef) isValidBVCron(bv *BuildVariant, proposedCron time.Time, now time.Time) (bool, error) {
	return p.isValidCron(proposedCron, now, func(mostRecentCommit *Version) bool {
		for _, bvStatus := range mostRecentCommit.BuildVariants {
			if bvStatus.BuildVariant != bv.Name {
				continue
			}

			return p.isActiveCronTimeRange(bvStatus.ActivateAt, now)
		}
		return false
	})
}

// isValidBVCron checks if a proposed time to activate a cron. A cron scheduled
// to activate in the future is always valid, but if the cron is scheduled to
// run in the past, that mean it will run immediately. Crons scheduled for the
// past are only valid if they've recently passed the proposed cron time and
// there's no conflicting cron that will activate or has already activated.
func (p *ProjectRef) isValidCron(proposedCron time.Time, now time.Time, isDuplicateCron func(mostRecentCommit *Version) bool) (bool, error) {
	if !p.isActiveCronTimeRange(proposedCron, now) {
		return false, nil
	}

	// Cron is scheduled in the past (i.e. it will activate immediately), but
	// it's within the allowed delay. Check that no other conflicting cron was
	// already scheduled to activate in that delay period - if there is a cron
	// that is activated/will activate soon, then this cron conflicts and is
	// therefore invalid.
	mostRecentCommit, err := VersionFindOne(VersionByMostRecentNonIgnored(p.Id, now))
	if err != nil {
		return false, errors.Wrap(err, "getting most recent commit version")
	}
	if mostRecentCommit == nil {
		return true, nil
	}

	return !isDuplicateCron(mostRecentCommit), nil
}

// GetActivationTimeForTask returns the time at which this task should next be
// activated. The version create time is used to determine the next activation
// time, except in situations where using the version create time would produce
// conflicts such as duplicate cron runs.
func (p *ProjectRef) GetActivationTimeForTask(t *BuildVariantTaskUnit, versionCreateTime time.Time, now time.Time) (time.Time, error) {
	// if we don't want to activate the task, set batchtime to the zero time
	if !utility.FromBoolTPtr(t.Activate) || t.IsDisabled() {
		return utility.ZeroTime, nil
	}
	if t.CronBatchTime != "" {
		// Prefer to schedule the cron activation time based on the version
		// create time.
		proposedCron, err := GetNextCronTime(versionCreateTime, t.CronBatchTime)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "getting next cron time")
		}
		isValidCron, err := p.isValidTaskCron(t, proposedCron, now)
		if err != nil {
			return time.Time{}, errors.Wrapf(err, "checking if proposed cron %s for task '%s' in build variant '%s' is valid", proposedCron.String(), t.Name, t.Variant)
		}
		if isValidCron {
			return proposedCron, nil
		}

		// If the cron is scheduled too far in the past or will conflict with
		// another cron activation, skip it and run the cron in the future
		// instead.
		return GetNextCronTime(now, t.CronBatchTime)
	}
	// If activated explicitly set to true and we don't have batchtime, then we want to just activate now
	if utility.FromBoolPtr(t.Activate) && t.BatchTime == nil {
		return time.Now(), nil
	}

	lastActivated, err := VersionFindOne(VersionByLastTaskActivation(p.Id, t.Variant, t.Name).WithFields(VersionBuildVariantsKey))
	if err != nil {
		return versionCreateTime, errors.Wrap(err, "finding version")
	}
	if lastActivated == nil {
		return versionCreateTime, nil
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
	return versionCreateTime, nil
}

// isValidBVCron checks is a build variant cron is valid.
func (p *ProjectRef) isValidTaskCron(bvtu *BuildVariantTaskUnit, proposedCron time.Time, now time.Time) (bool, error) {
	return p.isValidCron(proposedCron, now, func(mostRecentCommit *Version) bool {
		for _, bvStatus := range mostRecentCommit.BuildVariants {
			if bvStatus.BuildVariant != bvtu.Variant {
				continue
			}

			for _, taskStatus := range bvStatus.BatchTimeTasks {
				if taskStatus.TaskName != bvtu.Name {
					continue
				}

				return p.isActiveCronTimeRange(taskStatus.ActivateAt, now)
			}
		}
		return false
	})
}

// GetGithubProjectConflicts returns any potential conflicts; i.e. regardless of whether or not
// p has something enabled, returns the project identifiers that it _would_ conflict with if it did.
func (p *ProjectRef) GetGithubProjectConflicts() (GithubProjectConflicts, error) {
	res := GithubProjectConflicts{}
	// return early for projects that don't need to consider conflicts
	if p.Owner == "" || p.Repo == "" || p.Branch == "" {
		return res, nil
	}

	matchingProjects, err := FindMergedEnabledProjectRefsByRepoAndBranch(p.Owner, p.Repo, p.Branch)
	if err != nil {
		return res, errors.Wrap(err, "getting conflicting projects")
	}

	for _, conflictingRef := range matchingProjects {
		// If this is the same project ref or the potentially conflicting ref is going to inherit
		// from this ref it is not comflicting.
		if conflictingRef.Id == p.Id || conflictingRef.RepoRefId == p.Id {
			continue
		}
		if conflictingRef.IsPRTestingEnabled() {
			res.PRTestingIdentifiers = append(res.PRTestingIdentifiers, conflictingRef.Identifier)
		}
		if conflictingRef.CommitQueue.IsEnabled() {
			res.CommitQueueIdentifiers = append(res.CommitQueueIdentifiers, conflictingRef.Identifier)
		}
		if conflictingRef.IsGithubChecksEnabled() {
			res.CommitCheckIdentifiers = append(res.CommitCheckIdentifiers, conflictingRef.Identifier)
		}
	}
	return res, nil
}

// shouldValidateTotalProjectLimit will return true if:
// - we are creating a new project
// - the original project was disabled
func shouldValidateTotalProjectLimit(isNewProject bool, originalMergedRef *ProjectRef) bool {
	return isNewProject || !originalMergedRef.Enabled
}

// shouldValidateOwnerRepoLimit will return true if:
// - we are creating a new project
// - the original project was disabled
// - the owner or repo has changed
// - the owner/repo is not part of exception
func shouldValidateOwnerRepoLimit(isNewProject bool, config *evergreen.Settings, originalMergedRef, mergedRefToValidate *ProjectRef) bool {
	return (shouldValidateTotalProjectLimit(isNewProject, originalMergedRef) ||
		originalMergedRef.Owner != mergedRefToValidate.Owner || originalMergedRef.Repo != mergedRefToValidate.Repo) &&
		!config.ProjectCreation.IsExceptionToRepoLimit(mergedRefToValidate.Owner, mergedRefToValidate.Repo)
}

// ValidateEnabledProjectsLimit takes in a the original and new merged project refs and validates project limits,
// assuming the given project is going to be enabled.
// Returns a status code and error if we are already at limit with enabled projects.
func ValidateEnabledProjectsLimit(projectId string, config *evergreen.Settings, originalMergedRef, mergedRefToValidate *ProjectRef) (int, error) {
	if config.ProjectCreation.TotalProjectLimit == 0 || config.ProjectCreation.RepoProjectLimit == 0 {
		return http.StatusOK, nil
	}

	isNewProject := originalMergedRef == nil
	catcher := grip.NewBasicCatcher()
	if shouldValidateTotalProjectLimit(isNewProject, originalMergedRef) {
		allEnabledProjects, err := GetNumberOfEnabledProjects()
		if err != nil {
			return http.StatusInternalServerError, errors.Wrap(err, "getting number of enabled projects")
		}
		if allEnabledProjects >= config.ProjectCreation.TotalProjectLimit {
			catcher.Errorf("total enabled project limit of %d reached", config.ProjectCreation.TotalProjectLimit)
		}
	}

	if shouldValidateOwnerRepoLimit(isNewProject, config, originalMergedRef, mergedRefToValidate) {
		enabledOwnerRepoProjects, err := GetNumberOfEnabledProjectsForOwnerRepo(mergedRefToValidate.Owner, mergedRefToValidate.Repo)
		if err != nil {
			return http.StatusInternalServerError, errors.Wrapf(err, "getting number of projects for '%s/%s'", mergedRefToValidate.Owner, mergedRefToValidate.Repo)
		}
		if enabledOwnerRepoProjects >= config.ProjectCreation.RepoProjectLimit {
			catcher.Errorf("enabled project limit of %d reached for '%s/%s'", config.ProjectCreation.RepoProjectLimit, mergedRefToValidate.Owner, mergedRefToValidate.Repo)
		}
	}

	if catcher.HasErrors() {
		return http.StatusBadRequest, catcher.Resolve()
	}
	return http.StatusOK, nil
}

// ValidateProjectRefAndSetDefaults validates the project ref and sets default values.
// Should only be called on enabled project refs or repo refs.
func (p *ProjectRef) ValidateOwnerAndRepo(validOrgs []string) error {
	// verify input and webhooks
	if p.Owner == "" || p.Repo == "" {
		return errors.New("no owner/repo specified")
	}

	return validateOwner(p.Owner, validOrgs)
}

func validateOwner(owner string, validOrgs []string) error {
	if len(validOrgs) > 0 && !utility.StringSliceContains(validOrgs, owner) {
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
		return errors.Wrap(err, "counting other project refs")
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
	catcher.Wrap(err, "updating projects")
	_, err = db.UpdateAll(RepoRefCollection, bson.M{RepoRefAdminsKey: bson.M{"$ne": nil}}, repoUpdate)
	catcher.Wrap(err, "updating repos")
	return catcher.Resolve()
}

func (p *ProjectRef) MakeRestricted() error {
	rm := evergreen.GetEnvironment().RoleManager()
	// remove from the unrestricted branch project scope (if it exists)
	if p.UseRepoSettings() {
		scopeId := GetUnrestrictedBranchProjectsScope(p.RepoRefId)
		if err := rm.RemoveResourceFromScope(scopeId, p.Id); err != nil {
			return errors.Wrap(err, "removing resource from unrestricted branches scope")
		}
	}

	if err := rm.RemoveResourceFromScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "removing project '%s' from list of unrestricted projects", p.Id)
	}
	if err := rm.AddResourceToScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "adding project '%s' to list of restricted projects", p.Id)
	}

	return nil
}

func (p *ProjectRef) MakeUnrestricted() error {
	rm := evergreen.GetEnvironment().RoleManager()
	// remove from the unrestricted branch project scope (if it exists)
	if p.UseRepoSettings() {
		scopeId := GetUnrestrictedBranchProjectsScope(p.RepoRefId)
		if err := rm.AddResourceToScope(scopeId, p.Id); err != nil {
			return errors.Wrap(err, "adding resource to unrestricted branches scope")
		}
	}

	if err := rm.RemoveResourceFromScope(evergreen.RestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "removing project '%s' from list of restricted projects", p.Id)
	}
	if err := rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, p.Id); err != nil {
		return errors.Wrapf(err, "adding project '%s' to list of unrestricted projects", p.Id)
	}
	return nil
}

// UpdateAdminRoles returns true if any admins have been modified/removed, regardless of errors.
func (p *ProjectRef) UpdateAdminRoles(toAdd, toRemove []string) (bool, error) {
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return false, nil
	}
	rm := evergreen.GetEnvironment().RoleManager()
	role, err := rm.FindRoleWithPermissions(evergreen.ProjectResourceType, []string{p.Id}, adminPermissions)
	if err != nil {
		return false, errors.Wrap(err, "finding role with admin permissions")
	}
	if role == nil {
		return false, errors.Errorf("no admin role for project '%s' found", p.Id)
	}
	viewRole := ""
	allBranchAdmins := []string{}
	if p.RepoRefId != "" {
		allBranchAdmins, err = FindBranchAdminsForRepo(p.RepoRefId)
		if err != nil {
			return false, errors.Wrapf(err, "finding branch admins for repo '%s'", p.RepoRefId)
		}
		viewRole = GetViewRepoRole(p.RepoRefId)
	}

	catcher := grip.NewBasicCatcher()
	for _, addedUser := range toAdd {
		adminUser, err := user.FindOneById(addedUser)
		if err != nil {
			catcher.Wrapf(err, "finding user '%s'", addedUser)
			p.removeFromAdminsList(addedUser)
			continue
		}
		if adminUser == nil {
			catcher.Errorf("no user '%s' found", addedUser)
			p.removeFromAdminsList(addedUser)
			continue
		}
		if err = adminUser.AddRole(role.ID); err != nil {
			catcher.Wrapf(err, "adding role '%s' to user '%s'", role.ID, addedUser)
			p.removeFromAdminsList(addedUser)
			continue
		}
		if viewRole != "" {
			if err = adminUser.AddRole(viewRole); err != nil {
				catcher.Wrapf(err, "adding role '%s' to user '%s'", viewRole, addedUser)
				continue
			}
		}
	}
	for _, removedUser := range toRemove {
		adminUser, err := user.FindOneById(removedUser)
		if err != nil {
			catcher.Wrapf(err, "finding user '%s'", removedUser)
			continue
		}
		if adminUser == nil {
			continue
		}

		if err = adminUser.RemoveRole(role.ID); err != nil {
			catcher.Wrapf(err, "removing role '%s' from user '%s'", role.ID, removedUser)
			p.Admins = append(p.Admins, removedUser)
			continue
		}
		if viewRole != "" && !utility.StringSliceContains(allBranchAdmins, adminUser.Id) {
			if err = adminUser.RemoveRole(viewRole); err != nil {
				catcher.Wrapf(err, "removing role '%s' from user '%s'", viewRole, removedUser)
				continue
			}
		}
	}
	return true, errors.Wrap(catcher.Resolve(), "updating some admin roles")
}

func (p *ProjectRef) removeFromAdminsList(user string) {
	for i, name := range p.Admins {
		if name == user {
			p.Admins = append(p.Admins[:i], p.Admins[i+1:]...)
		}
	}
}

func (p *ProjectRef) AuthorizedForGitTag(ctx context.Context, githubUser, token, owner, repo string) bool {
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

	return thirdparty.IsUserInGithubTeam(ctx, p.GitTagAuthorizedTeams, p.Owner, githubUser, token, owner, repo)
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
		args := []string{"git", "clone", "-b", p.Branch, fmt.Sprintf("https://github.com/%s/%s.git", p.Owner, p.Repo), opts.Directory}
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

// UpdateNextPeriodicBuild updates the periodic build run time for either the project
// or repo ref depending on where it's defined.
func UpdateNextPeriodicBuild(projectId string, definition *PeriodicBuildDefinition) error {
	var nextRunTime time.Time
	var err error
	baseTime := definition.NextRunTime
	if utility.IsZeroTime(baseTime) {
		baseTime = time.Now()
	}
	if definition.IntervalHours > 0 {
		nextRunTime = baseTime.Add(time.Duration(definition.IntervalHours) * time.Hour)
	} else {
		nextRunTime, err = GetNextCronTime(baseTime, definition.Cron)
		if err != nil {
			return errors.Wrap(err, "getting next run time with cron")
		}
	}
	// Get the branch project on its own so we can determine where to update the run time.
	projectRef, err := FindBranchProjectRef(projectId)
	if err != nil {
		return errors.Wrap(err, "finding branch project")
	}
	if projectRef == nil {
		return errors.Errorf("project '%s' not found", projectId)
	}

	collection := ProjectRefCollection
	buildsKey := projectRefPeriodicBuildsKey
	documentIdKey := ProjectRefIdKey
	idToUpdate := projectRef.Id

	// If periodic builds aren't defined for the project, see if it's part of the repo and update there instead.
	if projectRef.PeriodicBuilds == nil && projectRef.UseRepoSettings() {
		repoRef, err := FindOneRepoRef(projectRef.RepoRefId)
		if err != nil {
			return err
		}
		if repoRef == nil {
			return errors.Errorf("repo '%s' not found", projectRef.RepoRefId)
		}
		for _, d := range repoRef.PeriodicBuilds {
			if d.ID == definition.ID {
				collection = RepoRefCollection
				buildsKey = RepoRefPeriodicBuildsKey
				documentIdKey = RepoRefIdKey
				idToUpdate = projectRef.RepoRefId
			}
		}
	}

	filter := bson.M{
		documentIdKey: idToUpdate,
		buildsKey: bson.M{
			"$elemMatch": bson.M{
				"id": definition.ID,
			},
		},
	}
	update := bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(buildsKey, "$", "next_run_time"): nextRunTime,
		},
	}

	return errors.Wrapf(db.Update(collection, filter, update), "updating collection '%s'", collection)
}

func (p *ProjectRef) CommitQueueIsOn() error {
	catcher := grip.NewBasicCatcher()
	if !p.Enabled {
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
	t, err := task.FindOneId(taskId)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task '%s'", taskId)
	}
	if t == nil {
		return nil, errors.Errorf("task '%s' not found", taskId)
	}
	pRef, err := FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project '%s'", t.Project)
	}
	if pRef == nil {
		return nil, errors.Errorf("project ref '%s' doesn't exist", t.Project)
	}
	return pRef, nil
}

func GetSetupScriptForTask(ctx context.Context, taskId string) (string, error) {
	conf, err := evergreen.GetConfig(ctx)
	if err != nil {
		return "", errors.Wrap(err, "can't get evergreen configuration")
	}
	token, err := conf.GetGithubOauthToken()
	if err != nil {
		return "", errors.Wrap(err, "getting GitHub token")
	}

	pRef, err := GetProjectRefForTask(taskId)
	if err != nil {
		return "", errors.Wrap(err, "getting project")
	}

	if pRef.SpawnHostScriptPath == "" {
		return "", nil
	}
	configFile, err := thirdparty.GetGithubFile(ctx, token, pRef.Owner, pRef.Repo, pRef.SpawnHostScriptPath, pRef.Branch)
	if err != nil {
		return "", errors.Wrapf(err,
			"fetching spawn host script for project '%s' at path '%s'", pRef.Identifier, pRef.SpawnHostScriptPath)
	}
	fileContents, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return "", errors.Wrapf(err,
			"unable to spawn host script for project '%s' at path '%s'", pRef.Identifier, pRef.SpawnHostScriptPath)
	}

	return string(fileContents), nil
}

func (t *TriggerDefinition) Validate(downstreamProject string) error {
	upstreamProject, err := FindBranchProjectRef(t.Project)
	if err != nil {
		return errors.Wrapf(err, "finding upstream project '%s'", t.Project)
	}
	if upstreamProject == nil {
		return errors.Errorf("project '%s' not found", t.Project)
	}
	if upstreamProject.Id == downstreamProject {
		return errors.New("a project cannot trigger itself")
	}

	// should be saved using its ID, in case the user used the project's identifier
	t.Project = upstreamProject.Id
	if t.Level != ProjectTriggerLevelBuild && t.Level != ProjectTriggerLevelTask && t.Level != ProjectTriggerLevelPush {
		return errors.Errorf("invalid level: %s", t.Level)
	}
	if t.Status != "" && t.Status != evergreen.TaskFailed && t.Status != evergreen.TaskSucceeded {
		return errors.Errorf("invalid status: %s", t.Status)
	}
	_, regexErr := regexp.Compile(t.BuildVariantRegex)
	if regexErr != nil {
		return errors.Wrapf(regexErr, "invalid variant regex '%s'", t.BuildVariantRegex)
	}
	_, regexErr = regexp.Compile(t.TaskRegex)
	if regexErr != nil {
		return errors.Wrapf(regexErr, "invalid task regex '%s'", t.TaskRegex)
	}
	if t.ConfigFile == "" {
		return errors.New("must provide a config file")
	}
	if t.DefinitionID == "" {
		t.DefinitionID = utility.RandomString()
	}
	return nil
}

func GetMessageForPatch(patchID string) (string, error) {
	requestedPatch, err := patch.FindOneId(patchID)
	if err != nil {
		return "", errors.Wrap(err, "finding patch")
	}
	if requestedPatch == nil {
		return "", errors.New("no patch found")
	}
	project, err := FindMergedProjectRef(requestedPatch.Project, requestedPatch.Version, true)
	if err != nil {
		return "", errors.Wrap(err, "finding project for patch")
	}
	if project == nil {
		return "", errors.New("patch has nonexistent project")
	}

	return project.CommitQueue.Message, nil
}

// ValidateContainers inspects the list of containers defined in the project YAML and checks that each
// are properly configured, and that their definitions can coexist with what is defined for container sizes
// on the project admin page.
func ValidateContainers(ecsConf evergreen.ECSConfig, pRef *ProjectRef, containers []Container) error {
	catcher := grip.NewSimpleCatcher()

	projVars, err := FindMergedProjectVars(pRef.Id)
	if err != nil {
		return errors.Wrapf(err, "getting project vars for project '%s'", pRef.Id)
	}
	var expansions *util.Expansions
	if projVars != nil {
		expansions = util.NewExpansions(projVars.Vars)
	}

	for _, container := range containers {
		image := container.Image
		if expansions != nil {
			image, err = expansions.ExpandString(container.Image)
			catcher.Wrap(err, "expanding container image")
		}
		catcher.Add(container.System.Validate())
		if container.Resources != nil {
			catcher.Add(container.Resources.Validate(ecsConf))
		}
		var containerSize *ContainerResources
		for _, size := range pRef.ContainerSizeDefinitions {
			if size.Name == container.Size {
				containerSize = &size
				break
			}
		}
		if containerSize != nil {
			catcher.Add(containerSize.Validate(ecsConf))
		}
		catcher.ErrorfWhen(container.Size != "" && containerSize == nil, "container size '%s' not found", container.Size)

		if container.Credential != "" {
			var matchingSecret *ContainerSecret
			for _, cs := range pRef.ContainerSecrets {
				if cs.Name == container.Credential {
					matchingSecret = &cs
					break
				}
			}
			catcher.ErrorfWhen(matchingSecret == nil, "credential '%s' is not defined in project settings", container.Credential)
			catcher.ErrorfWhen(matchingSecret != nil && matchingSecret.Type != ContainerSecretRepoCreds, "container credential named '%s' exists but is not valid for use as a repository credential", container.Credential)
		}
		catcher.NewWhen(container.Size != "" && container.Resources != nil, "size and resources cannot both be defined")
		catcher.NewWhen(container.Size == "" && container.Resources == nil, "either size or resources must be defined")
		catcher.NewWhen(container.Image == "", "image must be defined")
		catcher.NewWhen(container.WorkingDir == "", "working directory must be defined")
		catcher.NewWhen(container.Name == "", "name must be defined")
		catcher.ErrorfWhen(len(ecsConf.AllowedImages) > 0 && !util.HasAllowedImageAsPrefix(image, ecsConf.AllowedImages), "image '%s' not allowed", image)
	}
	return catcher.Resolve()
}

// Validate that essential ContainerSystem fields are properly defined and no data contradictions exist.
func (c ContainerSystem) Validate() error {
	catcher := grip.NewSimpleCatcher()
	if c.OperatingSystem != "" {
		catcher.Add(c.OperatingSystem.Validate())
	}
	if c.CPUArchitecture != "" {
		catcher.Add(c.CPUArchitecture.Validate())
	}
	if c.OperatingSystem == evergreen.WindowsOS {
		catcher.Add(c.WindowsVersion.Validate())
	}
	catcher.NewWhen(c.OperatingSystem == evergreen.LinuxOS && c.WindowsVersion != "", "cannot specify windows version when OS is linux")
	return catcher.Resolve()
}

// Validate that essential ContainerResources fields are properly defined.
func (c ContainerResources) Validate(ecsConf evergreen.ECSConfig) error {
	catcher := grip.NewSimpleCatcher()
	catcher.NewWhen(c.CPU <= 0, "container resource CPU must be a positive integer")
	catcher.NewWhen(c.MemoryMB <= 0, "container resource memory MB must be a positive integer")

	catcher.ErrorfWhen(ecsConf.MaxCPU > 0 && c.CPU > ecsConf.MaxCPU, "CPU cannot exceed maximum global limit of %d CPU units", ecsConf.MaxCPU)
	catcher.ErrorfWhen(ecsConf.MaxMemoryMB > 0 && c.MemoryMB > ecsConf.MaxMemoryMB, "memory cannot exceed maximum global limit of %d MB", ecsConf.MaxMemoryMB)

	return catcher.Resolve()
}

// Validate that essential container secret fields are properly defined for a
// new secret.
func (c ContainerSecret) Validate() error {
	catcher := grip.NewSimpleCatcher()
	catcher.Add(c.Type.Validate())
	catcher.ErrorfWhen(c.Name == "", "must specify name for new container secret")
	catcher.ErrorfWhen(c.Value == "", "must specify value for new container secret")
	return catcher.Resolve()
}

var validTriggerStatuses = []string{"", AllStatuses, evergreen.VersionSucceeded, evergreen.VersionFailed}

func ValidateTriggerDefinition(definition patch.PatchTriggerDefinition, parentProject string) (patch.PatchTriggerDefinition, error) {
	if definition.ChildProject == parentProject {
		return definition, errors.New("a project cannot trigger itself")
	}

	childProjectId, err := GetIdForProject(definition.ChildProject)
	if err != nil {
		return definition, errors.Wrapf(err, "finding child project '%s'", definition.ChildProject)
	}

	if !utility.StringSliceContains(validTriggerStatuses, definition.Status) {
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
			aliases, err = FindAliasInProjectRepoOrConfig(definition.ChildProject, specifier.PatchAlias)
			if err != nil {
				return definition, errors.Wrap(err, "fetching aliases for project")
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
	catcher.NewWhen(d.IntervalHours <= 0 && d.Cron == "", "interval must be a positive integer or cron must be defined")
	catcher.NewWhen(d.IntervalHours > 0 && d.Cron != "", "can't define both cron and interval")
	catcher.NewWhen(d.ConfigFile == "", "a config file must be specified")
	if d.Cron != "" {
		_, err := getCronParserSchedule(d.Cron)
		catcher.Wrap(err, "parsing cron")
	}

	if d.ID == "" {
		d.ID = utility.RandomString()
	}
	return catcher.Resolve()
}

// IsWebhookConfigured retrieves webhook configuration from the project settings.
func IsWebhookConfigured(project string, version string) (evergreen.WebHook, bool, error) {
	projectRef, err := FindMergedProjectRef(project, version, true)
	if err != nil || projectRef == nil {
		return evergreen.WebHook{}, false, errors.Errorf("finding merged project ref for project '%s'", project)
	}
	webHook := projectRef.TaskAnnotationSettings.FileTicketWebhook
	if webHook.Endpoint != "" {
		return webHook, true, nil
	} else {
		return evergreen.WebHook{}, false, nil
	}
}

func GetUpstreamProjectName(triggerID, triggerType string) (string, error) {
	if triggerID == "" || triggerType == "" {
		return "", nil
	}
	var projectID string
	if triggerType == ProjectTriggerLevelTask {
		upstreamTask, err := task.FindOneId(triggerID)
		if err != nil {
			return "", errors.Wrap(err, "finding upstream task")
		}
		if upstreamTask == nil {
			return "", errors.New("upstream task not found")
		}
		projectID = upstreamTask.Project
	} else if triggerType == ProjectTriggerLevelBuild {
		upstreamBuild, err := build.FindOneId(triggerID)
		if err != nil {
			return "", errors.Wrap(err, "finding upstream build")
		}
		if upstreamBuild == nil {
			return "", errors.New("upstream build not found")
		}
		projectID = upstreamBuild.Project
	}
	upstreamProject, err := FindBranchProjectRef(projectID)
	if err != nil {
		return "", errors.Wrap(err, "finding upstream project")
	}
	if upstreamProject == nil {
		return "", errors.New("upstream project not found")
	}
	return upstreamProject.DisplayName, nil
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
				}},
				{"$or": []bson.M{
					{
						bsonutil.GetDottedKeyName(projectRefCommitQueueKey, commitQueueEnabledKey):    true,
						bsonutil.GetDottedKeyName(projectRefCommitQueueKey, commitQueueMergeQueueKey): bson.M{"$ne": MergeQueueGitHub},
					},
					{
						bsonutil.GetDottedKeyName(projectRefCommitQueueKey, commitQueueEnabledKey):             nil,
						bsonutil.GetDottedKeyName("repo_ref", RepoRefCommitQueueKey, commitQueueEnabledKey):    true,
						bsonutil.GetDottedKeyName("repo_ref", RepoRefCommitQueueKey, commitQueueMergeQueueKey): bson.M{"$ne": MergeQueueGitHub},
					},
				}},
			}},
		},
		{"$project": bson.M{
			ProjectRefIdKey: 1,
		}},
	}
}

var lookupRepoStep = bson.M{"$lookup": bson.M{
	"from":         RepoRefCollection,
	"localField":   ProjectRefRepoRefIdKey,
	"foreignField": RepoRefIdKey,
	"as":           "repo_ref",
}}

// ContainerSecretCache implements the cocoa.SecretCache to provide a cache to
// store secrets in the DB's project ref.
type ContainerSecretCache struct{}

// Put sets the external ID for a project ref's container secret by its name.
func (c ContainerSecretCache) Put(_ context.Context, sc cocoa.SecretCacheItem) error {
	externalNameKey := bsonutil.GetDottedKeyName(projectRefContainerSecretsKey, containerSecretExternalNameKey)
	externalIDKey := bsonutil.GetDottedKeyName(projectRefContainerSecretsKey, containerSecretExternalIDKey)
	externalIDUpdateKey := bsonutil.GetDottedKeyName(projectRefContainerSecretsKey, "$", containerSecretExternalIDKey)
	return db.Update(ProjectRefCollection, bson.M{
		externalNameKey: sc.Name,
		externalIDKey: bson.M{
			"$in": []interface{}{"", sc.ID},
		},
	}, bson.M{
		"$set": bson.M{
			externalIDUpdateKey: sc.ID,
		},
	})
}

// Delete deletes a container secret from the project ref by its external
// identifier.
func (c ContainerSecretCache) Delete(_ context.Context, externalID string) error {
	externalIDKey := bsonutil.GetDottedKeyName(projectRefContainerSecretsKey, containerSecretExternalIDKey)
	err := db.Update(ProjectRefCollection, bson.M{
		externalIDKey: externalID,
	}, bson.M{
		"$pull": bson.M{
			projectRefContainerSecretsKey: bson.M{
				containerSecretExternalIDKey: externalID,
			},
		},
	})
	if adb.ResultsNotFound(err) {
		return nil
	}

	return err
}

// ContainerSecretTag is the tag used to track container secrets.
const ContainerSecretTag = "evergreen-tracked"

// GetTag returns the tag used for tracking cloud container secrets.
func (c ContainerSecretCache) GetTag() string {
	return ContainerSecretTag
}

// Constants related to secrets stored in Secrets Manager.
const (
	// internalSecretNamespace is the namespace for secrets that are
	// Evergreen-internal (such as the pod secret).
	internalSecretNamespace = "evg-internal"
	// repoCredsSecretName is the namespace for repository credentials.
	repoCredsSecretName = "repo-creds"
)

// makeContainerSecretName creates a Secrets Manager secret name namespaced
// within the given project ID.
func makeContainerSecretName(smConf evergreen.SecretsManagerConfig, projectID, name string) string {
	return strings.Join([]string{strings.TrimRight(smConf.SecretPrefix, "/"), "project", projectID, name}, "/")
}

// makeInternalContainerSecretName creates a Secrets Manager secret name
// namespaced by the given project ID for Evergreen-internal purposes.
func makeInternalContainerSecretName(smConf evergreen.SecretsManagerConfig, projectID, name string) string {
	return makeContainerSecretName(smConf, projectID, fmt.Sprintf("%s/%s", internalSecretNamespace, name))
}

// makeRepoCredsSecretName creates a Secrets Manager secret name namespaced by
// the given project ID for use as a repository credential.
func makeRepoCredsContainerSecretName(smConf evergreen.SecretsManagerConfig, projectID, name string) string {
	return makeContainerSecretName(smConf, projectID, fmt.Sprintf("%s/%s", repoCredsSecretName, name))
}

// ValidateContainerSecrets checks that the project-level container secrets to
// be added/updated are valid and sets default values where necessary. It
// returns the validated and merged container secrets, including the unmodified
// secrets, the modified secrets, and the new secrets to create.
func ValidateContainerSecrets(settings *evergreen.Settings, projectID string, original, toUpdate []ContainerSecret) ([]ContainerSecret, error) {
	combined := make([]ContainerSecret, len(original))
	_ = copy(combined, original)

	catcher := grip.NewBasicCatcher()
	podSecrets := make(map[string]bool)
	for _, originalSecret := range original {
		if originalSecret.Type == ContainerSecretPodSecret {
			podSecrets[originalSecret.Name] = true
		}
	}
	for _, updatedSecret := range toUpdate {
		name := updatedSecret.Name

		if updatedSecret.Type == ContainerSecretPodSecret {
			podSecrets[name] = true
		}

		idx := -1
		for i := 0; i < len(original); i++ {
			if original[i].Name == name {
				idx = i
				break
			}
		}

		if idx != -1 {
			existingSecret := combined[idx]
			// If updating an existing secret, only allow the value to be
			// updated.
			catcher.ErrorfWhen(updatedSecret.Type != "" && updatedSecret.Type != existingSecret.Type, "container secret '%s' type cannot be changed from '%s' to '%s'", name, existingSecret.Type, updatedSecret.Type)
			catcher.ErrorfWhen(updatedSecret.ExternalID != "" && updatedSecret.ExternalID != existingSecret.ExternalID, "container secret '%s' external ID cannot be changed from '%s' to '%s'", name, existingSecret.ExternalID, existingSecret.ExternalID)
			catcher.ErrorfWhen(updatedSecret.ExternalName != "" && updatedSecret.ExternalName != existingSecret.ExternalName, "container secret '%s' external name cannot be changed from '%s' to '%s'", name, existingSecret.ExternalName, updatedSecret.ExternalName)
			existingSecret.Value = updatedSecret.Value
			combined[idx] = existingSecret
			continue
		}

		catcher.Wrapf(updatedSecret.Validate(), "invalid new container secret '%s'", name)

		// New secrets that have to be created should not have their external
		// name and ID decided by the user. The external name is controlled by
		// Evergreen (and set here) and the external ID is determined by the
		// secret storage service (and set when the secret is actually stored).
		extName, err := newContainerSecretExternalName(settings.Providers.AWS.Pod.SecretsManager, projectID, updatedSecret)
		catcher.Add(err)
		updatedSecret.ExternalName = extName
		updatedSecret.ExternalID = ""

		combined = append(combined, updatedSecret)
	}

	catcher.ErrorfWhen(len(podSecrets) > 1, "a project can have at most one pod secret but tried to create %d pod secrets total", len(podSecrets))

	return combined, catcher.Resolve()
}

func newContainerSecretExternalName(smConf evergreen.SecretsManagerConfig, projectID string, secret ContainerSecret) (string, error) {
	switch secret.Type {
	case ContainerSecretPodSecret:
		return makeInternalContainerSecretName(smConf, projectID, secret.Name), nil
	case ContainerSecretRepoCreds:
		return makeRepoCredsContainerSecretName(smConf, projectID, secret.Name), nil
	default:
		return "", errors.Errorf("unrecognized secret type '%s' for container secret '%s'", secret.Type, secret.Name)
	}
}

// ProjectCanDispatchTask returns a boolean indicating if the task can be
// dispatched based on the project ref's settings and optionally includes a
// particular reason that the task can or cannot be dispatched.
func ProjectCanDispatchTask(pRef *ProjectRef, t *task.Task) (canDispatch bool, reason string) {
	// GitHub PR tasks are still allowed to run for disabled hidden projects.
	if !pRef.Enabled {
		// GitHub PR tasks are still allowed to run for disabled hidden
		// projects.
		if t.Requester == evergreen.GithubPRRequester && pRef.IsHidden() {
			reason = "GitHub PRs are allowed to run tasks for disabled hidden projects"
		} else {
			return false, "project is disabled"
		}
	}

	if pRef.IsDispatchingDisabled() {
		return false, "task dispatching is disabled for its project"
	}

	if t.IsPatchRequest() && pRef.IsPatchingDisabled() {
		return false, "patch testing is disabled for its project"
	}

	return true, reason
}
