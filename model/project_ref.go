package model

import (
	"fmt"
	"math"
	"net/url"
	"regexp"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// The ProjectRef struct contains general information, independent of any
// revision control system, needed to track a given project
type ProjectRef struct {
	Owner              string `bson:"owner_name" json:"owner_name" yaml:"owner"`
	Repo               string `bson:"repo_name" json:"repo_name" yaml:"repo"`
	Branch             string `bson:"branch_name" json:"branch_name" yaml:"branch"`
	RepoKind           string `bson:"repo_kind" json:"repo_kind" yaml:"repokind"`
	Enabled            bool   `bson:"enabled" json:"enabled" yaml:"enabled"`
	Private            bool   `bson:"private" json:"private" yaml:"private"`
	BatchTime          int    `bson:"batch_time" json:"batch_time" yaml:"batchtime"`
	RemotePath         string `bson:"remote_path" json:"remote_path" yaml:"remote_path"`
	Identifier         string `bson:"identifier" json:"identifier" yaml:"identifier"`
	DisplayName        string `bson:"display_name" json:"display_name" yaml:"display_name"`
	LocalConfig        string `bson:"local_config" json:"local_config" yaml:"local_config"`
	DeactivatePrevious bool   `bson:"deactivate_previous" json:"deactivate_previous" yaml:"deactivate_previous"`

	// TracksPushEvents, if true indicates that Repotracker is triggered by
	// Github PushEvents for this project, instead of the Repotracker runner
	TracksPushEvents bool `bson:"tracks_push_events" json:"tracks_push_events" yaml:"tracks_push_events"`

	PRTestingEnabled bool              `bson:"pr_testing_enabled" json:"pr_testing_enabled" yaml:"pr_testing_enabled"`
	CommitQueue      CommitQueueParams `bson:"commit_queue" json:"commit_queue" yaml:"commit_queue"`

	//Tracked determines whether or not the project is discoverable in the UI
	Tracked          bool `bson:"tracked" json:"tracked"`
	PatchingDisabled bool `bson:"patching_disabled" json:"patching_disabled"`

	// Admins contain a list of users who are able to access the projects page.
	Admins []string `bson:"admins" json:"admins"`

	NotifyOnBuildFailure bool `bson:"notify_on_failure" json:"notify_on_failure"`

	// RepoDetails contain the details of the status of the consistency
	// between what is in GitHub and what is in Evergreen
	RepotrackerError *RepositoryErrorDetails `bson:"repotracker_error" json:"repotracker_error"`

	// List of regular expressions describing files to ignore when caching historical test results
	FilesIgnoredFromCache []string `bson:"files_ignored_from_cache,omitempty" json:"files_ignored_from_cache,omitempty"`
	DisabledStatsCache    bool     `bson:"disabled_stats_cache,omitempty" json:"disabled_stats_cache,omitempty"`

	Triggers []TriggerDefinition `bson:"triggers,omitempty" json:"triggers,omitempty"`
}

type CommitQueueParams struct {
	Enabled      bool   `bson:"enabled" json:"enabled"`
	MergeMethod  string `bson:"merge_method" json:"merge_method"`
	PatchType    string `bson:"patch_type" json:"patch_type"`
	StatusAction string `bson:"status_action" json:"status_action"`
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
	ProjectRefOwnerKey              = bsonutil.MustHaveTag(ProjectRef{}, "Owner")
	ProjectRefRepoKey               = bsonutil.MustHaveTag(ProjectRef{}, "Repo")
	ProjectRefBranchKey             = bsonutil.MustHaveTag(ProjectRef{}, "Branch")
	ProjectRefRepoKindKey           = bsonutil.MustHaveTag(ProjectRef{}, "RepoKind")
	ProjectRefEnabledKey            = bsonutil.MustHaveTag(ProjectRef{}, "Enabled")
	ProjectRefPrivateKey            = bsonutil.MustHaveTag(ProjectRef{}, "Private")
	ProjectRefBatchTimeKey          = bsonutil.MustHaveTag(ProjectRef{}, "BatchTime")
	ProjectRefIdentifierKey         = bsonutil.MustHaveTag(ProjectRef{}, "Identifier")
	ProjectRefDisplayNameKey        = bsonutil.MustHaveTag(ProjectRef{}, "DisplayName")
	ProjectRefDeactivatePreviousKey = bsonutil.MustHaveTag(ProjectRef{}, "DeactivatePrevious")
	ProjectRefRemotePathKey         = bsonutil.MustHaveTag(ProjectRef{}, "RemotePath")
	ProjectRefTrackedKey            = bsonutil.MustHaveTag(ProjectRef{}, "Tracked")
	ProjectRefLocalConfig           = bsonutil.MustHaveTag(ProjectRef{}, "LocalConfig")
	ProjectRefRepotrackerError      = bsonutil.MustHaveTag(ProjectRef{}, "RepotrackerError")
	ProjectRefFilesIgnoredFromCache = bsonutil.MustHaveTag(ProjectRef{}, "FilesIgnoredFromCache")
	ProjectRefDisabledStatsCache    = bsonutil.MustHaveTag(ProjectRef{}, "DisabledStatsCache")
	ProjectRefAdminsKey             = bsonutil.MustHaveTag(ProjectRef{}, "Admins")
	projectRefTracksPushEventsKey   = bsonutil.MustHaveTag(ProjectRef{}, "TracksPushEvents")
	projectRefPRTestingEnabledKey   = bsonutil.MustHaveTag(ProjectRef{}, "PRTestingEnabled")
	projectRefCommitQueueKey        = bsonutil.MustHaveTag(ProjectRef{}, "CommitQueue")
	projectRefPatchingDisabledKey   = bsonutil.MustHaveTag(ProjectRef{}, "PatchingDisabled")
	projectRefNotifyOnFailureKey    = bsonutil.MustHaveTag(ProjectRef{}, "NotifyOnBuildFailure")
	projectRefTriggersKey           = bsonutil.MustHaveTag(ProjectRef{}, "Triggers")

	projectRefCommitQueueEnabledKey = bsonutil.MustHaveTag(CommitQueueParams{}, "Enabled")
	projectRefTriggerProjectKey     = bsonutil.MustHaveTag(TriggerDefinition{}, "Project")
)

const (
	ProjectRefCollection     = "project_ref"
	ProjectTriggerLevelTask  = "task"
	ProjectTriggerLevelBuild = "build"
)

func (projectRef *ProjectRef) Insert() error {
	return db.Insert(ProjectRefCollection, projectRef)
}

func (projectRef *ProjectRef) Update() error {
	return db.Update(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdentifierKey: projectRef.Identifier,
		},
		projectRef,
	)
}

// FindOneProjectRef gets a project ref given the owner name, the repo
// name and the project name
func FindOneProjectRef(identifier string) (*ProjectRef, error) {
	projectRef := &ProjectRef{}
	err := db.FindOne(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdentifierKey: identifier,
		},
		db.NoProjection,
		db.NoSort,
		projectRef,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return projectRef, err
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
	return projectRef, err
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
	return projectRefs, err
}

// FindProjectRefsByRepoAndBranch finds ProjectRefs with matching repo/branch
// that are enabled and setup for PR testing
func FindProjectRefsByRepoAndBranch(owner, repoName, branch string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
			ProjectRefOwnerKey:   owner,
			ProjectRefRepoKey:    repoName,
			ProjectRefBranchKey:  branch,
			ProjectRefEnabledKey: true,
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

	return projectRefs, err
}

func FindDownstreamProjects(project string) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}

	err := db.FindAll(
		ProjectRefCollection,
		bson.M{
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
	return projectRefs, err
}

// FindOneProjectRefByRepoAndBranch finds a signle ProjectRef with matching
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

	return &projectRefs[target], nil
}

func FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(owner, repo, branch string) (*ProjectRef, error) {
	projRef := &ProjectRef{}
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
		projRef,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "can't query for project with commit queue. owner: %s, repo: %s, branch: %s", owner, repo, branch)
	}

	return projRef, nil
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
	return projectRefs, nil
}

// FindProjectRefs returns limit refs starting at project identifier key
// in the sortDir direction
func FindProjectRefs(key string, limit int, sortDir int, isAuthenticated bool) ([]ProjectRef, error) {
	projectRefs := []ProjectRef{}
	filter := bson.M{}
	if !isAuthenticated {
		filter[ProjectRefPrivateKey] = false
	}
	sortSpec := ProjectIdentifierKey

	if sortDir < 0 {
		sortSpec = "-" + sortSpec
		filter[ProjectIdentifierKey] = bson.M{"$lt": key}
	} else {
		filter[ProjectIdentifierKey] = bson.M{"$gte": key}
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
	return projectRefs, err
}

// UntrackStaleProjectRefs sets all project_refs in the db not in the array
// of project identifiers to "untracked."
func UntrackStaleProjectRefs(activeProjects []string) error {
	_, err := db.UpdateAll(
		ProjectRefCollection,
		bson.M{ProjectRefIdentifierKey: bson.M{
			"$nin": activeProjects,
		}},
		bson.M{"$set": bson.M{
			ProjectRefTrackedKey: false,
		}},
	)
	return err
}

// Upsert updates the project ref in the db if an entry already exists,
// overwriting the existing ref. If no project ref exists, one is created
func (projectRef *ProjectRef) Upsert() error {
	_, err := db.Upsert(
		ProjectRefCollection,
		bson.M{
			ProjectRefIdentifierKey: projectRef.Identifier,
		},
		bson.M{
			"$set": bson.M{
				ProjectRefRepoKindKey:           projectRef.RepoKind,
				ProjectRefEnabledKey:            projectRef.Enabled,
				ProjectRefPrivateKey:            projectRef.Private,
				ProjectRefBatchTimeKey:          projectRef.BatchTime,
				ProjectRefOwnerKey:              projectRef.Owner,
				ProjectRefRepoKey:               projectRef.Repo,
				ProjectRefBranchKey:             projectRef.Branch,
				ProjectRefDisplayNameKey:        projectRef.DisplayName,
				ProjectRefDeactivatePreviousKey: projectRef.DeactivatePrevious,
				ProjectRefTrackedKey:            projectRef.Tracked,
				ProjectRefRemotePathKey:         projectRef.RemotePath,
				ProjectRefTrackedKey:            projectRef.Tracked,
				ProjectRefLocalConfig:           projectRef.LocalConfig,
				ProjectRefRepotrackerError:      projectRef.RepotrackerError,
				ProjectRefFilesIgnoredFromCache: projectRef.FilesIgnoredFromCache,
				ProjectRefDisabledStatsCache:    projectRef.DisabledStatsCache,
				ProjectRefAdminsKey:             projectRef.Admins,
				projectRefTracksPushEventsKey:   projectRef.TracksPushEvents,
				projectRefPRTestingEnabledKey:   projectRef.PRTestingEnabled,
				projectRefCommitQueueKey:        projectRef.CommitQueue,
				projectRefPatchingDisabledKey:   projectRef.PatchingDisabled,
				projectRefNotifyOnFailureKey:    projectRef.NotifyOnBuildFailure,
				projectRefTriggersKey:           projectRef.Triggers,
			},
		},
	)
	return err
}

// ProjectRef returns a string representation of a ProjectRef
func (projectRef *ProjectRef) String() string {
	return projectRef.Identifier
}

// GetBatchTime returns the Batch Time of the ProjectRef
func (p *ProjectRef) GetBatchTime(variant *BuildVariant) int {
	var val int = p.BatchTime
	if variant.BatchTime != nil {
		val = *variant.BatchTime
	}

	// BatchTime is in minutes, but it is stored/used internally as
	// nanoseconds. We need to cap this value to Int32 to prevent an
	// overflow/wrap around to negative values of time.Duration
	if val > math.MaxInt32 {
		return math.MaxInt32
	} else {
		return val
	}
}

// Location generates and returns the ssh hostname and path to the repo.
func (projectRef *ProjectRef) Location() (string, error) {
	if projectRef.Owner == "" {
		return "", errors.Errorf("No owner in project ref: %v", projectRef.Identifier)
	}
	if projectRef.Repo == "" {
		return "", errors.Errorf("No repo in project ref: %v", projectRef.Identifier)
	}
	return fmt.Sprintf("git@github.com:%v/%v.git", projectRef.Owner, projectRef.Repo), nil
}

// HTTPLocation creates a url.URL for HTTPS checkout of a Github repository
func (projectRef *ProjectRef) HTTPLocation() (*url.URL, error) {
	if projectRef.Owner == "" {
		return nil, errors.Errorf("No owner in project ref: %s", projectRef.Identifier)
	}
	if projectRef.Repo == "" {
		return nil, errors.Errorf("No repo in project ref: %s", projectRef.Identifier)
	}

	return &url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   fmt.Sprintf("/%s/%s.git", projectRef.Owner, projectRef.Repo),
	}, nil
}

func (p *ProjectRef) IsAdmin(userID string, settings evergreen.Settings) bool {
	return util.StringSliceContains(p.Admins, userID) || util.StringSliceContains(settings.SuperUsers, userID)
}

func (t TriggerDefinition) Validate(parentProject string) error {
	upstreamProject, err := FindOneProjectRef(t.Project)
	if err != nil {
		return errors.Wrapf(err, "error finding upstream project %s", t.Project)
	}
	if upstreamProject == nil {
		return errors.Errorf("project %s not found", t.Project)
	}
	if upstreamProject.Identifier == parentProject {
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
