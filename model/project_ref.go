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
	Id                      string                         `bson:"_id" json:"id" yaml:"id"`
	DisplayName             string                         `bson:"display_name" json:"display_name" yaml:"display_name"`
	Enabled                 *bool                          `bson:"enabled" json:"enabled" yaml:"enabled"`
	Private                 *bool                          `bson:"private" json:"private" yaml:"private"`
	Restricted              *bool                          `bson:"restricted" json:"restricted" yaml:"restricted"`
	Owner                   string                         `bson:"owner_name" json:"owner_name" yaml:"owner"`
	Repo                    string                         `bson:"repo_name" json:"repo_name" yaml:"repo"`
	Branch                  string                         `bson:"branch_name" json:"branch_name" yaml:"branch"`
	RemotePath              string                         `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	PatchingDisabled        *bool                          `bson:"patching_disabled" json:"patching_disabled"`
	RepotrackerDisabled     *bool                          `bson:"repotracker_disabled" json:"repotracker_disabled" yaml:"repotracker_disabled"`
	DispatchingDisabled     *bool                          `bson:"dispatching_disabled" json:"dispatching_disabled" yaml:"dispatching_disabled"`
	PRTestingEnabled        *bool                          `bson:"pr_testing_enabled" json:"pr_testing_enabled" yaml:"pr_testing_enabled"`
	GithubChecksEnabled     *bool                          `bson:"github_checks_enabled" json:"github_checks_enabled" yaml:"github_checks_enabled"`
	BatchTime               int                            `bson:"batch_time" json:"batch_time" yaml:"batchtime"`
	DeactivatePrevious      *bool                          `bson:"deactivate_previous" json:"deactivate_previous" yaml:"deactivate_previous"`
	DefaultLogger           string                         `bson:"default_logger" json:"default_logger" yaml:"default_logger"`
	NotifyOnBuildFailure    *bool                          `bson:"notify_on_failure" json:"notify_on_failure"`
	Triggers                []TriggerDefinition            `bson:"triggers" json:"triggers"`
	PatchTriggerAliases     []patch.PatchTriggerDefinition `bson:"patch_trigger_aliases" json:"patch_trigger_aliases"`
	PeriodicBuilds          []PeriodicBuildDefinition      `bson:"periodic_builds" json:"periodic_builds"`
	CedarTestResultsEnabled *bool                          `bson:"cedar_test_results_enabled" json:"cedar_test_results_enabled" yaml:"cedar_test_results_enabled"`
	CommitQueue             CommitQueueParams              `bson:"commit_queue" json:"commit_queue" yaml:"commit_queue"`

	// Admins contain a list of users who are able to access the projects page.
	Admins []string `bson:"admins" json:"admins"`

	// SpawnHostScriptPath is a path to a script to optionally be run by users on hosts triggered from tasks.
	SpawnHostScriptPath string `bson:"spawn_host_script_path" json:"spawn_host_script_path" yaml:"spawn_host_script_path"`

	// Identifier must be unique, but is modifiable. Used by users.
	Identifier string `bson:"identifier" json:"identifier" yaml:"identifier"`

	// TracksPushEvents, if true indicates that Repotracker is triggered by Github PushEvents for this project.
	TracksPushEvents bool `bson:"tracks_push_events" json:"tracks_push_events" yaml:"tracks_push_events"`

	// TaskSync holds settings for synchronizing task directories to S3.
	TaskSync TaskSyncOptions `bson:"task_sync" json:"task_sync" yaml:"task_sync"`

	// GitTagAuthorizedUsers contains a list of users who are able to create versions from git tags.
	GitTagAuthorizedUsers []string `bson:"git_tag_authorized_users" json:"git_tag_authorized_users"`
	GitTagAuthorizedTeams []string `bson:"git_tag_authorized_teams" json:"git_tag_authorized_teams"`
	GitTagVersionsEnabled *bool    `bson:"git_tag_versions_enabled" json:"git_tag_versions_enabled"`

	// RepoDetails contain the details of the status of the consistency
	// between what is in GitHub and what is in Evergreen
	RepotrackerError *RepositoryErrorDetails `bson:"repotracker_error" json:"repotracker_error"`

	// List of regular expressions describing files to ignore when caching historical test results
	FilesIgnoredFromCache []string `bson:"files_ignored_from_cache" json:"files_ignored_from_cache"`
	DisabledStatsCache    *bool    `bson:"disabled_stats_cache" json:"disabled_stats_cache"`

	// List of commands
	WorkstationConfig WorkstationConfig `bson:"workstation_config,omitempty" json:"workstation_config,omitempty"`

	// The following fields are used by Evergreen and are not discoverable.
	// Hidden determines whether or not the project is discoverable/tracked in the UI
	Hidden *bool `bson:"hidden" json:"hidden"`

	// This is a temporary flag to enable individual projects to use repo settings
	UseRepoSettings bool   `bson:"use_repo_settings" json:"use_repo_settings" yaml:"use_repo_settings"`
	RepoRefId       string `bson:"repo_ref_id" json:"repo_ref_id" yaml:"repo_ref_id"`
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
	GitClone      bool                      `bson:"git_clone" json:"git_clone"`
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
	ProjectRefFilesIgnoredFromCache      = bsonutil.MustHaveTag(ProjectRef{}, "FilesIgnoredFromCache")
	ProjectRefDisabledStatsCache         = bsonutil.MustHaveTag(ProjectRef{}, "DisabledStatsCache")
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
	projectRefPeriodicBuildsKey          = bsonutil.MustHaveTag(ProjectRef{}, "PeriodicBuilds")
	projectRefWorkstationConfigKey       = bsonutil.MustHaveTag(ProjectRef{}, "WorkstationConfig")

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

func (p *CommitQueueParams) IsEnabled() bool {
	return utility.FromBoolPtr(p.Enabled)
}

func (ts *TaskSyncOptions) IsPatchEnabled() bool {
	return utility.FromBoolPtr(ts.PatchEnabled)
}

func (ts *TaskSyncOptions) IsConfigEnabled() bool {
	return utility.FromBoolPtr(ts.ConfigEnabled)
}

const (
	ProjectRefCollection     = "project_ref"
	ProjectTriggerLevelTask  = "task"
	ProjectTriggerLevelBuild = "build"
	intervalPrefix           = "@every"
	maxBatchTime             = 153722867 // math.MaxInt64 / 60 / 1_000_000_000
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
	p.Id = mgobson.NewObjectId().Hex()
	err := db.Insert(ProjectRefCollection, p)
	if err != nil {
		return errors.Wrap(err, "Error inserting distro")
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

func (p *ProjectRef) AddToRepoScope(user *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	if p.RepoRefId == "" {
		repoRef, err := FindRepoRefByOwnerAndRepo(p.Owner, p.Repo)
		if err != nil {
			return errors.Wrapf(err, "error finding repo ref '%s'", p.RepoRefId)
		}
		if repoRef == nil {
			repoRef = &RepoRef{ProjectRef{
				Id:      mgobson.NewObjectId().Hex(),
				Admins:  []string{user.Username()},
				Owner:   p.Owner,
				Repo:    p.Repo,
				Enabled: utility.TruePtr(),
			}}
			// creates scope and give user admin access to repo
			if err = repoRef.Add(user); err != nil {
				return errors.Wrapf(err, "problem adding new repo repo ref for '%s/%s'", p.Owner, p.Repo)
			}
			newProjectVars := ProjectVars{Id: repoRef.Id}

			if err = newProjectVars.Insert(); err != nil {
				return errors.Wrap(err, "error inserting blank project variables for repo")
			}
		}
		p.RepoRefId = repoRef.Id
	} else {
		// if the repo exists, then the scope also exists, so add this project ID to the scope, and give the user repo admin access
		repoRole := GetRepoAdminRole(p.RepoRefId)
		if !utility.StringSliceContains(user.Roles(), repoRole) {
			if err := user.AddRole(repoRole); err != nil {
				return errors.Wrapf(err, "error adding admin role to repo '%s'", user.Username())
			}
			if err := addAdminToRepo(p.RepoRefId, user.Username()); err != nil {
				return errors.Wrapf(err, "error adding user as repo admin")
			}
		}
		if err := rm.AddResourceToScope(GetRepoScope(p.RepoRefId), p.Id); err != nil {
			return errors.Wrapf(err, "error adding resource to repo '%s' scope", p.RepoRefId)
		}
	}

	return errors.Wrapf(addViewRepoPermissionsToBranchAdmins(p.RepoRefId, p.Admins),
		"error giving branch '%s' admins view permission for repo '%s'", p.Id, p.RepoRefId)
}

func (p *ProjectRef) RemoveFromRepoScope() error {
	if p.RepoRefId == "" {
		return nil
	}
	rm := evergreen.GetEnvironment().RoleManager()
	return rm.RemoveResourceFromScope(GetRepoScope(p.RepoRefId), p.Id)
}

func (p *ProjectRef) AddPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	// if the branch is restricted, then it's not accessible to repo admins, so we don't use the repo scope
	parentScope := evergreen.UnrestrictedProjectsScope
	if p.UseRepoSettings {
		parentScope = GetRepoScope(p.RepoRefId)
	}
	if p.IsRestricted() {
		parentScope = evergreen.AllProjectsScope
	}

	// add scope for the branch-level project configurations
	newScope := gimlet.Scope{
		ID:          fmt.Sprintf("project_%s", p.Id),
		Resources:   []string{p.Id},
		Name:        p.Id,
		Type:        evergreen.ProjectResourceType,
		ParentScope: parentScope,
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

// FindOneProjectRef gets a project ref given the project identifier.
// This returns only branch-level settings; to include repo settings, use FindMergedProjectRef.
func FindOneProjectRef(identifier string) (*ProjectRef, error) {
	return findOneProjectRefQ(byId(identifier))
}

// FindMergedProjectRef also finds the repo ref settings and merges relevant fields.
func FindMergedProjectRef(identifier string) (*ProjectRef, error) {
	pRef, err := FindOneProjectRef(identifier)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding project ref '%s'", identifier)
	}
	if pRef == nil {
		return nil, errors.Errorf("project ref '%s' does not exist", identifier)
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

func FindIdForProject(identifier string) (string, error) {
	pRef, err := findOneProjectRefQ(byId(identifier).WithFields(ProjectRefIdKey))
	if err != nil {
		return "", err
	}
	if pRef == nil {
		return "", errors.Errorf("project '%s' does not exist", identifier)
	}
	return pRef.Id, nil
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
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefHiddenKey: bson.M{"$ne": true},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)
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

func FindAllMergedTrackedProjectRefsWithRepoInfo() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefHiddenKey: bson.M{"$ne": true},
			ProjectRefOwnerKey:  bson.M{"$exists": true, "$ne": ""},
			ProjectRefRepoKey:   bson.M{"$exists": true, "$ne": ""},
			ProjectRefBranchKey: bson.M{"$exists": true, "$ne": ""},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
}

// FindAllMergedProjectRefs returns all project refs in the db, with repo ref information merged
func FindAllMergedProjectRefs() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)
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

// FindMergedProjectRefsByRepoAndBranch finds ProjectRefs with matching repo/branch
// that are enabled, and merges repo information.
func FindMergedProjectRefsByRepoAndBranch(owner, repoName, branch string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	pipeline := []bson.M{{"$match": byOwnerRepoAndBranch(owner, repoName, branch)}}
	pipeline = append(pipeline, projectRefPipelineForValueIsBool(ProjectRefEnabledKey, RepoRefEnabledKey, true)...)
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

// FindOneProjectRefByRepoAndBranch finds a single ProjectRef with matching
// repo/branch that is enabled and setup for PR testing.
func FindOneProjectRefByRepoAndBranchWithPRTesting(owner, repo, branch string) (*ProjectRef, error) {
	projectRefs, err := FindMergedProjectRefsByRepoAndBranch(owner, repo, branch)
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

	grip.Debug(message.Fields{
		"message": "no matching project ref with pr testing enabled",
		"owner":   owner,
		"repo":    repo,
		"branch":  branch,
	})
	return nil, nil
}

// FindOneProjectRef finds the project ref for this owner/repo/branch that has the commit queue enabled.
// There should only ever be one project for the query because we only enable commit queue if
// no other project ref with the same specification has it enabled.

func FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	projectRefs, err := FindMergedProjectRefsByRepoAndBranch(owner, repo, branch)
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

func FindMergedEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	pipeline := []bson.M{byOwnerAndRepo(owner, repo)}
	pipeline = append(pipeline, projectRefPipelineForValueIsBool(ProjectRefEnabledKey, RepoRefEnabledKey, true)...)
	err := db.Aggregate(ProjectRefCollection, pipeline, &projectRefs)
	if err != nil {
		return nil, err
	}

	return addLoggerAndRepoSettingsToProjects(projectRefs)
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

	err := db.FindAll(
		ProjectRefCollection,
		filter,
		db.NoProjection,
		[]string{sortSpec},
		db.NoSkip,
		limit,
		&projectRefs,
	)

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
		},
		bson.M{
			"$set": bson.M{
				ProjectRefIdentifierKey:              projectRef.Identifier,
				ProjectRefRepoRefIdKey:               projectRef.RepoRefId,
				ProjectRefEnabledKey:                 projectRef.Enabled,
				ProjectRefPrivateKey:                 projectRef.Private,
				ProjectRefRestrictedKey:              projectRef.Restricted,
				ProjectRefBatchTimeKey:               projectRef.BatchTime,
				ProjectRefOwnerKey:                   projectRef.Owner,
				ProjectRefRepoKey:                    projectRef.Repo,
				ProjectRefBranchKey:                  projectRef.Branch,
				ProjectRefDisplayNameKey:             projectRef.DisplayName,
				ProjectRefDeactivatePreviousKey:      projectRef.DeactivatePrevious,
				ProjectRefRemotePathKey:              projectRef.RemotePath,
				ProjectRefHiddenKey:                  projectRef.Hidden,
				ProjectRefRepotrackerError:           projectRef.RepotrackerError,
				ProjectRefFilesIgnoredFromCache:      projectRef.FilesIgnoredFromCache,
				ProjectRefDisabledStatsCache:         projectRef.DisabledStatsCache,
				ProjectRefAdminsKey:                  projectRef.Admins,
				ProjectRefGitTagAuthorizedUsersKey:   projectRef.GitTagAuthorizedUsers,
				ProjectRefGitTagAuthorizedTeamsKey:   projectRef.GitTagAuthorizedTeams,
				projectRefTracksPushEventsKey:        projectRef.TracksPushEvents,
				projectRefDefaultLoggerKey:           projectRef.DefaultLogger,
				projectRefCedarTestResultsEnabledKey: projectRef.CedarTestResultsEnabled,
				projectRefPRTestingEnabledKey:        projectRef.PRTestingEnabled,
				projectRefGithubChecksEnabledKey:     projectRef.GithubChecksEnabled,
				projectRefGitTagVersionsEnabledKey:   projectRef.GitTagVersionsEnabled,
				projectRefUseRepoSettingsKey:         projectRef.UseRepoSettings,
				projectRefCommitQueueKey:             projectRef.CommitQueue,
				projectRefTaskSyncKey:                projectRef.TaskSync,
				projectRefPatchingDisabledKey:        projectRef.PatchingDisabled,
				projectRefRepotrackerDisabledKey:     projectRef.RepotrackerDisabled,
				projectRefDispatchingDisabledKey:     projectRef.DispatchingDisabled,
				projectRefNotifyOnFailureKey:         projectRef.NotifyOnBuildFailure,
				projectRefSpawnHostScriptPathKey:     projectRef.SpawnHostScriptPath,
				projectRefTriggersKey:                projectRef.Triggers,
				projectRefPatchTriggerAliasesKey:     projectRef.PatchTriggerAliases,
				projectRefPeriodicBuildsKey:          projectRef.PeriodicBuilds,
				projectRefWorkstationConfigKey:       projectRef.WorkstationConfig,
			},
		},
	)
	return err
}

// ProjectRef returns a string representation of a ProjectRef
func (projectRef *ProjectRef) String() string {
	return projectRef.Id
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
	if variant.CronBatchTime != "" {
		return GetActivationTimeWithCron(time.Now(), variant.CronBatchTime)
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
	if t.CronBatchTime != "" {
		return GetActivationTimeWithCron(time.Now(), t.CronBatchTime)
	}

	lastActivated, err := VersionFindOne(VersionByLastTaskActivation(p.Id, t.Variant, t.Name).WithFields(VersionBuildVariantsKey))
	if err != nil {
		return defaultRes, errors.Wrap(err, "error finding version")
	}
	if lastActivated == nil {
		return defaultRes, nil
	}

	for _, buildStatus := range lastActivated.BuildVariants {
		if buildStatus.BuildVariant != t.Variant || !buildStatus.Activated {
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
	catcher.Add(db.Update(ProjectRefCollection, bson.M{}, projectUpdate))
	catcher.Add(db.Update(RepoRefCollection, bson.M{}, repoUpdate))
	return catcher.Resolve()
}

func (p *ProjectRef) MakeRestricted(ctx context.Context) error {
	rm := evergreen.GetEnvironment().RoleManager()
	// attempt to remove the resource from the repo (which will also remove from its parent)
	// if the repo scope doesn't exist then we'll remove only from unrestricted

	scopeId := GetRepoScope(p.RepoRefId)
	scope, err := rm.GetScope(ctx, scopeId)
	if err != nil {
		return errors.Wrapf(err, "error looking for repo scope")
	}
	if scope != nil {
		return errors.Wrap(rm.RemoveResourceFromScope(scopeId, p.Id), "error removing resource from scope")
	}
	return errors.Wrapf(rm.RemoveResourceFromScope(evergreen.UnrestrictedProjectsScope, p.Id), "unable to remove %s from list of unrestricted projects", p.Id)

}

func (p *ProjectRef) MakeUnrestricted(ctx context.Context) error {
	rm := evergreen.GetEnvironment().RoleManager()
	// attempt to add the resource to the repo (which will also add to its parent)
	// if the repo scope doesn't exist then we'll add only to unrestricted

	scopeId := GetRepoScope(p.RepoRefId)
	scope, err := rm.GetScope(ctx, scopeId)
	if err != nil {
		return errors.Wrapf(err, "error looking for repo scope")
	}
	if scope != nil {
		return errors.Wrap(rm.AddResourceToScope(scopeId, p.Id), "error adding resource to scope")
	}
	return errors.Wrapf(rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, p.Id), "unable to add %s to list of unrestricted projects", p.Id)
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

	for _, addedUser := range toAdd {
		adminUser, err := user.FindOneById(addedUser)
		if err != nil {
			return errors.Wrapf(err, "error finding user '%s'", addedUser)
		}
		if adminUser == nil {
			return errors.Errorf("no user '%s' found", addedUser)
		}
		if err = adminUser.AddRole(role.ID); err != nil {
			return errors.Wrapf(err, "error adding role %s to user %s", role.ID, addedUser)
		}
		if viewRole != "" {
			if err = adminUser.AddRole(viewRole); err != nil {
				return errors.Wrapf(err, "error adding role %s to user %s", viewRole, addedUser)
			}
		}
	}
	for _, removedUser := range toRemove {
		adminUser, err := user.FindOneById(removedUser)
		if err != nil {
			return errors.Wrapf(err, "error finding user %s", removedUser)
		}
		if adminUser == nil {
			continue
		}

		if err = adminUser.RemoveRole(role.ID); err != nil {
			return errors.Wrapf(err, "error removing role %s from user %s", role.ID, removedUser)
		}
		if viewRole != "" && !utility.StringSliceContains(allBranchAdmins, adminUser.Id) {
			if err = adminUser.RemoveRole(viewRole); err != nil {
				return errors.Wrapf(err, "error removing role %s from user %s", viewRole, removedUser)
			}
		}
	}
	return nil
}

func (p *ProjectRef) AuthorizedForGitTag(ctx context.Context, user string, token string) bool {
	if utility.StringSliceContains(p.GitTagAuthorizedUsers, user) {
		return true
	}
	return thirdparty.IsUserInGithubTeam(ctx, p.GitTagAuthorizedTeams, p.Owner, user, token)
}

// GetProjectSetupCommands returns jasper commands for the project's configuration commands
// Stderr/Stdin are passed through to the commands as well as Stdout, when opts.Quiet is false
// The commands' working directories may not exist and need to be created before running the commands
func (p *ProjectRef) GetProjectSetupCommands(opts apimodels.WorkstationSetupCommandOptions) ([]*jasper.Command, error) {
	if len(p.WorkstationConfig.SetupCommands) == 0 && !p.WorkstationConfig.GitClone {
		return nil, errors.Errorf("no setup commands configured for project '%s'", p.Id)
	}

	baseDir := filepath.Join(opts.Directory, p.Id)
	cmds := []*jasper.Command{}

	if p.WorkstationConfig.GitClone {
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

	configFile, err := thirdparty.GetGithubFile(ctx, token, pRef.Owner, pRef.Repo, pRef.SpawnHostScriptPath, "")
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
	upstreamProject, err := FindOneProjectRef(t.Project)
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

	childProjectId, err := FindIdForProject(definition.ChildProject)
	if err != nil {
		return definition, errors.Wrapf(err, "error finding child project %s", definition.ChildProject)
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
	upstreamProject, err := FindOneProjectRef(projectID)
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
