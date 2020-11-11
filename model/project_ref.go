package model

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"go.mongodb.org/mongo-driver/bson"
)

// The ProjectRef struct contains general information, independent of any
// revision control system, needed to track a given project
type ProjectRef struct {
	// The following fields can be defined from both the branch and repo level.
	// Id is the unmodifiable unique ID for the configuration, used internally.
	Id                  string `bson:"_id" json:"id" yaml:"id"`
	DisplayName         string `bson:"display_name" json:"display_name" yaml:"display_name"`
	Enabled             bool   `bson:"enabled" json:"enabled" yaml:"enabled"`
	Private             bool   `bson:"private" json:"private" yaml:"private"`
	Restricted          bool   `bson:"restricted" json:"restricted" yaml:"restricted"`
	Owner               string `bson:"owner_name" json:"owner_name" yaml:"owner"`
	Repo                string `bson:"repo_name" json:"repo_name" yaml:"repo"`
	RemotePath          string `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	PatchingDisabled    bool   `bson:"patching_disabled" json:"patching_disabled"`
	RepotrackerDisabled bool   `bson:"repotracker_disabled" json:"repotracker_disabled" yaml:"repotracker_disabled"`
	DispatchingDisabled bool   `bson:"dispatching_disabled" json:"dispatching_disabled" yaml:"dispatching_disabled"`
	PRTestingEnabled    bool   `bson:"pr_testing_enabled" json:"pr_testing_enabled" yaml:"pr_testing_enabled"`

	// Admins contain a list of users who are able to access the projects page.
	Admins []string `bson:"admins" json:"admins"`

	// SpawnHostScriptPath is a path to a script to optionally be run by users on hosts triggered from tasks.
	SpawnHostScriptPath string `bson:"spawn_host_script_path" json:"spawn_host_script_path" yaml:"spawn_host_script_path"`

	// The following fields can be defined only at the branch level.
	Branch                  string                    `bson:"branch_name" json:"branch_name" yaml:"branch"`
	BatchTime               int                       `bson:"batch_time" json:"batch_time" yaml:"batchtime"`
	DeactivatePrevious      bool                      `bson:"deactivate_previous" json:"deactivate_previous" yaml:"deactivate_previous"`
	DefaultLogger           string                    `bson:"default_logger" json:"default_logger" yaml:"default_logger"`
	NotifyOnBuildFailure    bool                      `bson:"notify_on_failure" json:"notify_on_failure"`
	Triggers                []TriggerDefinition       `bson:"triggers,omitempty" json:"triggers,omitempty"`
	PeriodicBuilds          []PeriodicBuildDefinition `bson:"periodic_builds,omitempty" json:"periodic_builds,omitempty"`
	Tags                    []string                  `bson:"tags" json:"tags,omitempty" yaml:"tags,omitempty"`
	CedarTestResultsEnabled bool                      `bson:"cedar_test_results_enabled" json:"cedar_test_results_enabled" yaml:"cedar_test_results_enabled"`
	CommitQueue             CommitQueueParams         `bson:"commit_queue" json:"commit_queue" yaml:"commit_queue"`

	// Identifier must be unique, but is modifiable. Used by users.
	Identifier string `bson:"identifier" json:"identifier" yaml:"identifier"`

	// TracksPushEvents, if true indicates that Repotracker is triggered by Github PushEvents for this project.
	TracksPushEvents bool `bson:"tracks_push_events" json:"tracks_push_events" yaml:"tracks_push_events"`

	// TaskSync holds settings for synchronizing task directories to S3.
	TaskSync TaskSyncOptions `bson:"task_sync" json:"task_sync" yaml:"task_sync"`

	// GitTagAuthorizedUsers contains a list of users who are able to create versions from git tags.
	GitTagAuthorizedUsers []string `bson:"git_tag_authorized_users,omitempty" json:"git_tag_authorized_users,omitempty"`
	GitTagAuthorizedTeams []string `bson:"git_tag_authorized_teams,omitempty" json:"git_tag_authorized_teams,omitempty"`
	GitTagVersionsEnabled bool     `bson:"git_tag_versions_enabled" json:"git_tag_versions_enabled"`

	// RepoDetails contain the details of the status of the consistency
	// between what is in GitHub and what is in Evergreen
	RepotrackerError *RepositoryErrorDetails `bson:"repotracker_error" json:"repotracker_error"`

	// List of regular expressions describing files to ignore when caching historical test results
	FilesIgnoredFromCache []string `bson:"files_ignored_from_cache,omitempty" json:"files_ignored_from_cache,omitempty"`
	DisabledStatsCache    bool     `bson:"disabled_stats_cache" json:"disabled_stats_cache"`

	// List of commands
	WorkstationConfig WorkstationConfig `bson:"workstation_config,omitempty" json:"workstation_config,omitempty"`

	// The following fields are used by Evergreen and are not discoverable.
	RepoKind string `bson:"repo_kind" json:"repo_kind" yaml:"repokind"`
	//Tracked determines whether or not the project is discoverable in the UI
	Tracked bool `bson:"tracked" json:"tracked"`
}

type CommitQueueParams struct {
	Enabled     bool   `bson:"enabled" json:"enabled"`
	MergeMethod string `bson:"merge_method" json:"merge_method"`
	PatchType   string `bson:"patch_type" json:"patch_type"`
	Message     string `bson:"message,omitempty" json:"message,omitempty"`
}

// TaskSyncOptions contains information about which features are allowed for
// syncing task directories to S3.
type TaskSyncOptions struct {
	ConfigEnabled bool `bson:"config_enabled" json:"config_enabled"`
	PatchEnabled  bool `bson:"patch_enabled" json:"patch_enabled"`
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
	ProjectRefRepoKindKey                = bsonutil.MustHaveTag(ProjectRef{}, "RepoKind")
	ProjectRefEnabledKey                 = bsonutil.MustHaveTag(ProjectRef{}, "Enabled")
	ProjectRefPrivateKey                 = bsonutil.MustHaveTag(ProjectRef{}, "Private")
	ProjectRefRestrictedKey              = bsonutil.MustHaveTag(ProjectRef{}, "Restricted")
	ProjectRefBatchTimeKey               = bsonutil.MustHaveTag(ProjectRef{}, "BatchTime")
	ProjectRefIdentifierKey              = bsonutil.MustHaveTag(ProjectRef{}, "Identifier")
	ProjectRefDisplayNameKey             = bsonutil.MustHaveTag(ProjectRef{}, "DisplayName")
	ProjectRefDeactivatePreviousKey      = bsonutil.MustHaveTag(ProjectRef{}, "DeactivatePrevious")
	ProjectRefRemotePathKey              = bsonutil.MustHaveTag(ProjectRef{}, "RemotePath")
	ProjectRefTrackedKey                 = bsonutil.MustHaveTag(ProjectRef{}, "Tracked")
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
	projectRefGitTagVersionsEnabledKey   = bsonutil.MustHaveTag(ProjectRef{}, "GitTagVersionsEnabled")
	projectRefRepotrackerDisabledKey     = bsonutil.MustHaveTag(ProjectRef{}, "RepotrackerDisabled")
	projectRefCommitQueueKey             = bsonutil.MustHaveTag(ProjectRef{}, "CommitQueue")
	projectRefTaskSyncKey                = bsonutil.MustHaveTag(ProjectRef{}, "TaskSync")
	projectRefPatchingDisabledKey        = bsonutil.MustHaveTag(ProjectRef{}, "PatchingDisabled")
	projectRefDispatchingDisabledKey     = bsonutil.MustHaveTag(ProjectRef{}, "DispatchingDisabled")
	projectRefNotifyOnFailureKey         = bsonutil.MustHaveTag(ProjectRef{}, "NotifyOnBuildFailure")
	projectRefSpawnHostScriptPathKey     = bsonutil.MustHaveTag(ProjectRef{}, "SpawnHostScriptPath")
	projectRefTriggersKey                = bsonutil.MustHaveTag(ProjectRef{}, "Triggers")
	projectRefPeriodicBuildsKey          = bsonutil.MustHaveTag(ProjectRef{}, "PeriodicBuilds")
	projectRefTagsKey                    = bsonutil.MustHaveTag(ProjectRef{}, "Tags")
	projectRefWorkstationConfigKey       = bsonutil.MustHaveTag(ProjectRef{}, "WorkstationConfig")

	projectRefCommitQueueEnabledKey = bsonutil.MustHaveTag(CommitQueueParams{}, "Enabled")
	projectRefTriggerProjectKey     = bsonutil.MustHaveTag(TriggerDefinition{}, "Project")
)

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
	err := db.Insert(ProjectRefCollection, p)
	if err != nil {
		return errors.Wrap(err, "Error inserting distro")
	}
	return p.AddPermissions(creator)
}

func (p *ProjectRef) AddPermissions(creator *user.DBUser) error {
	rm := evergreen.GetEnvironment().RoleManager()
	catcher := grip.NewBasicCatcher()
	if !p.Restricted {
		catcher.Wrapf(rm.AddResourceToScope(evergreen.UnrestrictedProjectsScope, p.Id), "error adding project '%s' to list of unrestricted projects", p.Id)
	}
	catcher.Wrapf(rm.AddResourceToScope(evergreen.AllProjectsScope, p.Id), "error adding project '%s' to list of all projects", p.Id)
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	newScope := gimlet.Scope{
		ID:          fmt.Sprintf("project_%s", p.Id),
		Resources:   []string{p.Id},
		Name:        p.Id,
		Type:        evergreen.ProjectResourceType,
		ParentScope: evergreen.AllProjectsScope,
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

// FindOneProjectRef gets a project ref given the project identifier
func FindOneProjectRef(identifier string) (*ProjectRef, error) {
	return findOneProjectRefQ(byId(identifier))
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
	err := db.FindOne(
		ProjectRefCollection,
		bson.M{
			ProjectRefPrivateKey: false,
		},
		db.NoProjection,
		[]string{"-" + ProjectRefDisplayNameKey},
		projectRef,
	)

	projectRef.checkDefaultLogger()

	return projectRef, err
}

func FindTaggedProjectRefs(includeDisabled bool, tags ...string) ([]ProjectRef, error) {
	if len(tags) == 0 {
		return nil, errors.New("must specify one or more tags")
	}

	q := bson.M{}
	if !includeDisabled {
		q[ProjectRefEnabledKey] = true
	}

	if len(tags) == 1 {
		q[projectRefTagsKey] = tags[0]
	} else {
		q[projectRefTagsKey] = bson.M{"$in": tags}
	}

	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		q,
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, errors.WithStack(err)
	}

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, nil
}

// FindAllTrackedProjectRefs returns all project refs in the db
// that are currently being tracked (i.e. their project files
// still exist)
func FindAllTrackedProjectRefs() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{ProjectRefTrackedKey: true},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, err
}

func FindAllTrackedProjectRefsWithRepoInfo() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefTrackedKey: true,
			ProjectRefOwnerKey:   bson.M{"$exists": true, "$ne": ""},
			ProjectRefRepoKey:    bson.M{"$exists": true, "$ne": ""},
			ProjectRefBranchKey:  bson.M{"$exists": true, "$ne": ""},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
		&projectRefs,
	)

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, err
}

// FindAllProjectRefs returns all project refs in the db
func FindAllProjectRefs() ([]ProjectRef, error) {
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

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, err
}

func byOwnerRepoAndBranch(owner, repoName, branch string) db.Q {
	return db.Query(bson.M{
		ProjectRefOwnerKey:   owner,
		ProjectRefRepoKey:    repoName,
		ProjectRefBranchKey:  branch,
		ProjectRefEnabledKey: true,
	})
}

func byId(identifier string) db.Q {
	return db.Query(bson.M{
		"$or": []bson.M{
			{ProjectRefIdKey: identifier},
			{ProjectRefIdentifierKey: identifier},
		},
	})
}

// FindProjectRefsByRepoAndBranch finds ProjectRefs with matching repo/branch
// that are enabled and setup for PR testing
func FindProjectRefsByRepoAndBranch(owner, repoName, branch string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAllQ(ProjectRefCollection, byOwnerRepoAndBranch(owner, repoName, branch), &projectRefs)
	if err != nil {
		return nil, err
	}

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, err
}

func FindDownstreamProjects(project string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefEnabledKey: true,
			bsonutil.GetDottedKeyName(projectRefTriggersKey, projectRefTriggerProjectKey): project,
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

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, err
}

// FindOneProjectRefByRepoAndBranch finds a single ProjectRef with matching
// repo/branch that is enabled and setup for PR testing. If more than one
// is found, an error is returned
func FindOneProjectRefByRepoAndBranchWithPRTesting(owner, repo, branch string) (*ProjectRef, error) {
	projectRefs, err := FindProjectRefsByRepoAndBranch(owner, repo, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not fetch project ref for repo '%s/%s' with branch '%s'",
			owner, repo, branch)
	}
	l := len(projectRefs)
	target := 0
	if l > 1 {
		count := 0
		for i := range projectRefs {
			if projectRefs[i].PRTestingEnabled {
				target = i
				count += 1
			}
		}

		if count > 1 {
			err = errors.Errorf("attempt to fetch project ref for "+
				"'%s/%s' on branch '%s' found %d project refs, when 1 was expected",
				owner, repo, branch, count)
			return nil, err
		}

	}

	if l == 0 || !projectRefs[target].PRTestingEnabled {
		return nil, nil
	}

	if projectRefs[target].DefaultLogger == "" {
		projectRefs[target].checkDefaultLogger()
	}

	return &projectRefs[target], nil
}

// FindOneProjectRef finds the project ref for this owner/repo/branch that has the commit queue enabled.
// There should only ever be one project for the query because we only enable commit queue if
// no other project ref with the same specification has it enabled.
func FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	projectRef := &ProjectRef{}
	err := db.FindOne(
		ProjectRefCollection,
		bson.M{
			ProjectRefOwnerKey:  owner,
			ProjectRefRepoKey:   repo,
			ProjectRefBranchKey: branch,
			bsonutil.GetDottedKeyName(projectRefCommitQueueKey, projectRefCommitQueueEnabledKey): true,
		},
		db.NoProjection,
		db.NoSort,
		projectRef,
	)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "can't query for project with commit queue. owner: %s, repo: %s, branch: %s", owner, repo, branch)
	}

	projectRef.checkDefaultLogger()

	return projectRef, nil
}

func FindEnabledProjectRefsByOwnerAndRepo(owner, repo string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefEnabledKey: true,
			ProjectRefOwnerKey:   owner,
			ProjectRefRepoKey:    repo,
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

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, nil
}

func FindProjectRefsWithCommitQueueEnabled() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefEnabledKey: true,
			bsonutil.GetDottedKeyName(projectRefCommitQueueKey, projectRefCommitQueueEnabledKey): true,
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

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, nil
}

func FindPeriodicProjects() ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			projectRefPeriodicBuildsKey: bson.M{
				"$gt": bson.M{
					"$size": 0,
				},
			},
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

	for i := range projectRefs {
		projectRefs[i].checkDefaultLogger()
	}

	return projectRefs, nil
}

// FindProjectRefs returns limit refs starting at project identifier key
// in the sortDir direction
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
				ProjectRefRepoKindKey:                projectRef.RepoKind,
				ProjectRefEnabledKey:                 projectRef.Enabled,
				ProjectRefPrivateKey:                 projectRef.Private,
				ProjectRefRestrictedKey:              projectRef.Restricted,
				ProjectRefBatchTimeKey:               projectRef.BatchTime,
				ProjectRefOwnerKey:                   projectRef.Owner,
				ProjectRefRepoKey:                    projectRef.Repo,
				ProjectRefBranchKey:                  projectRef.Branch,
				ProjectRefDisplayNameKey:             projectRef.DisplayName,
				projectRefTagsKey:                    projectRef.Tags,
				ProjectRefDeactivatePreviousKey:      projectRef.DeactivatePrevious,
				ProjectRefTrackedKey:                 projectRef.Tracked,
				ProjectRefRemotePathKey:              projectRef.RemotePath,
				ProjectRefTrackedKey:                 projectRef.Tracked,
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
				projectRefGitTagVersionsEnabledKey:   projectRef.GitTagVersionsEnabled,
				projectRefCommitQueueKey:             projectRef.CommitQueue,
				projectRefTaskSyncKey:                projectRef.TaskSync,
				projectRefPatchingDisabledKey:        projectRef.PatchingDisabled,
				projectRefRepotrackerDisabledKey:     projectRef.RepotrackerDisabled,
				projectRefDispatchingDisabledKey:     projectRef.DispatchingDisabled,
				projectRefNotifyOnFailureKey:         projectRef.NotifyOnBuildFailure,
				projectRefSpawnHostScriptPathKey:     projectRef.SpawnHostScriptPath,
				projectRefTriggersKey:                projectRef.Triggers,
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

func (p *ProjectRef) RemoveTag(tag string) (bool, error) {
	newTags := []string{}
	for _, t := range p.Tags {
		if tag == t {
			continue
		}
		newTags = append(newTags, t)
	}
	if len(newTags) == len(p.Tags) {
		return false, nil
	}

	err := db.Update(
		ProjectRefCollection,
		bson.M{ProjectRefIdKey: p.Id},
		bson.M{"$pull": bson.M{projectRefTagsKey: tag}},
	)
	if adb.ResultsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "database error")
	}

	p.Tags = newTags

	return true, nil
}

func (p *ProjectRef) AddTags(tags ...string) (bool, error) {
	set := make(map[string]struct{}, len(p.Tags))
	for _, t := range p.Tags {
		set[t] = struct{}{}
	}
	toAdd := []string{}
	catcher := grip.NewBasicCatcher()
	for _, t := range tags {
		if _, ok := set[t]; ok {
			continue
		}
		catcher.ErrorfWhen(strings.Contains(t, ","),
			"cannot specify tags with a comma (,) [%s]", t)
		toAdd = append(toAdd, t)
	}
	if catcher.HasErrors() {
		return false, catcher.Resolve()
	}

	if len(toAdd) == 0 {
		return false, nil
	}

	err := db.Update(
		ProjectRefCollection,
		bson.M{ProjectRefIdKey: p.Id},
		bson.M{"$addToSet": bson.M{projectRefTagsKey: bson.M{"$each": toAdd}}},
	)
	if adb.ResultsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "database error")
	}

	p.Tags = append(p.Tags, toAdd...)

	return true, nil
}

func (p *ProjectRef) MakeRestricted() error {
	rm := evergreen.GetEnvironment().RoleManager()
	return errors.Wrapf(rm.RemoveResourceFromScope(evergreen.UnrestrictedProjectsScope, p.Id), "unable to remove %s from list of unrestricted projects", p.Id)
}

func (p *ProjectRef) MakeUnrestricted() error {
	rm := evergreen.GetEnvironment().RoleManager()
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
	for _, addedUser := range toAdd {
		adminUser, err := user.FindOneById(addedUser)
		if err != nil {
			return errors.Wrapf(err, "error finding user '%s'", addedUser)
		}
		if adminUser == nil {
			return errors.Errorf("no user '%s' found", addedUser)
		}
		if !utility.StringSliceContains(adminUser.Roles(), role.ID) {
			err = adminUser.AddRole(role.ID)
			if err != nil {
				return errors.Wrapf(err, "error adding role to user %s", addedUser)
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
		err = adminUser.RemoveRole(role.ID)
		if err != nil {
			return errors.Wrapf(err, "error removing role from user %s", removedUser)
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
	filter := bson.M{
		ProjectRefIdKey: p.Id,
		projectRefPeriodicBuildsKey: bson.M{
			"$elemMatch": bson.M{
				"id": definition,
			},
		},
	}
	update := bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(projectRefPeriodicBuildsKey, "$", "next_run_time"): nextRun,
		},
	}

	return db.Update(ProjectRefCollection, filter, update)
}

func (p *ProjectRef) CommitQueueIsOn() error {
	catcher := grip.NewBasicCatcher()
	if !p.Enabled {
		catcher.Add(errors.Errorf("project '%s' is disabled", p.Id))
	}
	if p.PatchingDisabled {
		catcher.Add(errors.Errorf("patching is disabled for project '%s'", p.Id))
	}
	if !p.CommitQueue.Enabled {
		catcher.Add(errors.Errorf("commit queue is disabled for project '%s'", p.Id))
	}

	return catcher.Resolve()
}

func GetProjectRefForTask(taskId string) (*ProjectRef, error) {
	projectId, err := task.FindProjectForTask(taskId)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project")
	}
	pRef, err := FindOneProjectRef(projectId)
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
			"error fetching spawn host script for '%s' at path '%s'", pRef.Id, pRef.SpawnHostScriptPath)
	}
	fileContents, err := base64.StdEncoding.DecodeString(*configFile.Content)
	if err != nil {
		return "", errors.Wrapf(err,
			"unable to spawn host script for '%s' at path '%s'", pRef.Id, pRef.SpawnHostScriptPath)
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
