package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	taskCollection       = "tasks"
	oldTaskCollection    = "old_tasks"
	taskVersionKey       = "version"
	taskDisplayOnlyKey   = "display_only"
	taskCostKey          = "cost"
	taskPredictedCostKey = "predicted_cost"
	taskOnDemandCostKey  = "on_demand_ec2_cost"
	taskAdjustedCostKey  = "adjusted_ec2_cost"
)

type Version struct {
	Id         string    `bson:"_id" json:"id,omitempty"`
	CreateTime time.Time `bson:"create_time" json:"create_time,omitempty"`
	StartTime  time.Time `bson:"start_time" json:"start_time,omitempty"`
	FinishTime time.Time `bson:"finish_time" json:"finish_time,omitempty"`
	Revision   string    `bson:"gitspec" json:"revision,omitempty"`
	// Author is a reference to the Evergreen user that authored
	// this commit, if they can be identified. This may refer to the user's
	// ID or their display name.
	Author string `bson:"author" json:"author,omitempty"`
	// AuthorID is an optional reference to the Evergreen user that authored
	// this commit, if they can be identified. This always refers to the user's
	// ID.
	AuthorID            string `bson:"author_id,omitempty" json:"author_id,omitempty"`
	AuthorEmail         string `bson:"author_email" json:"author_email,omitempty"`
	Message             string `bson:"message" json:"message,omitempty"`
	Status              string `bson:"status" json:"status,omitempty"`
	RevisionOrderNumber int    `bson:"order,omitempty" json:"order,omitempty"`
	Ignored             bool   `bson:"ignored" json:"ignored"`
	Owner               string `bson:"owner_name" json:"owner_name,omitempty"`
	Repo                string `bson:"repo_name" json:"repo_name,omitempty"`
	Branch              string `bson:"branch_name" json:"branch_name,omitempty"`
	// BuildVariants contains information about build variant activation. This
	// is not always loaded in version document queries because it can be large.
	// See (Version).GetBuildVariants to fetch this field.
	BuildVariants   []VersionBuildStatus `bson:"build_variants_status,omitempty" json:"build_variants_status,omitempty"`
	PeriodicBuildID string               `bson:"periodic_build_id,omitempty" json:"periodic_build_id,omitempty"`
	Aborted         bool                 `bson:"aborted,omitempty" json:"aborted,omitempty"`

	// This stores whether or not a version has tasks which were activated.
	// We use a bool ptr in order to to distinguish the unset value from the default value
	Activated *bool `bson:"activated,omitempty" json:"activated,omitempty"`

	// GitTags stores tags that were pushed to this version, while TriggeredByGitTag is for versions created by tags
	GitTags           []GitTag `bson:"git_tags,omitempty" json:"git_tags,omitempty"`
	TriggeredByGitTag GitTag   `bson:"triggered_by_git_tag,omitempty" json:"triggered_by_git_tag"`

	// Parameters stores user-defined parameters
	Parameters []patch.Parameter `bson:"parameters,omitempty" json:"parameters,omitempty"`
	// This is technically redundant, but a lot of code relies on it, so I'm going to leave it
	BuildIds []string `bson:"builds" json:"builds,omitempty"`

	Identifier string `bson:"identifier" json:"identifier,omitempty"`
	Remote     bool   `bson:"remote" json:"remote,omitempty"`
	RemotePath string `bson:"remote_path" json:"remote_path,omitempty"`
	// version requester - this is used to help tell the
	// reason this version was created. e.g. it could be
	// because the repotracker requested it (via tracking the
	// repository) or it was triggered by a developer
	// patch request
	Requester string `bson:"r" json:"requester,omitempty"`

	// child patches will store the id of the parent patch
	ParentPatchID     string `bson:"parent_patch_id" json:"parent_patch_id,omitempty"`
	ParentPatchNumber int    `bson:"parent_patch_number" json:"parent_patch_number,omitempty"`

	// version errors - this is used to keep track of any errors that were
	// encountered in the process of creating a version. If there are no errors
	// this field is omitted in the database
	Errors   []string `bson:"errors,omitempty" json:"errors,omitempty"`
	Warnings []string `bson:"warnings,omitempty" json:"warnings,omitempty"`

	SatisfiedTriggers []string `bson:"satisfied_triggers,omitempty" json:"satisfied_triggers,omitempty"`

	// Fields set if triggered by an upstream build
	// TriggerID is the ID of the entity that triggered the downstream version.
	TriggerID string `bson:"trigger_id,omitempty" json:"trigger_id,omitempty"`
	// TriggerType is the type of entity that triggered the downstream version.
	TriggerType string `bson:"trigger_type,omitempty" json:"trigger_type,omitempty"`
	// TriggerEvent is the event ID that triggered the downstream version.
	TriggerEvent string `bson:"trigger_event,omitempty" json:"trigger_event,omitempty"`
	// TriggerSHA is the SHA of the untracked commit that triggered the downstream version,
	// this field is only populated for push level triggers.
	TriggerSHA string `bson:"trigger_sha,omitempty" json:"trigger_sha,omitempty"`

	// this is only used for aggregations, and is not stored in the DB
	Builds []build.Build `bson:"build_variants,omitempty" json:"build_variants,omitempty"`

	// ProjectStorageMethod describes how the parser project for this version is
	// stored. If this is empty, the default storage method is StorageMethodDB.
	ProjectStorageMethod evergreen.ParserProjectStorageMethod `bson:"storage_method" json:"storage_method,omitempty"`
	// PreGenerationProjectStorageMethod describes how the cached parser project from before it was modified
	// by generate.tasks for this version is stored. If this is empty, the default storage method is StorageMethodDB.
	PreGenerationProjectStorageMethod evergreen.ParserProjectStorageMethod `bson:"pre_generation_storage_method" json:"pre_generation_storage_method,omitempty"`

	// Cost stores the aggregated actual cost (on-demand and adjusted components) of all execution tasks in the version.
	Cost cost.Cost `bson:"cost,omitempty" json:"cost,omitempty"`
	// PredictedCost stores the aggregated predicted cost derived from tasks' predicted_cost.
	PredictedCost cost.Cost `bson:"predicted_cost,omitempty" json:"predicted_cost,omitempty"`
}

func (v *Version) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(v) }
func (v *Version) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, v) }

const (
	defaultVersionLimit               = 20
	DefaultMainlineCommitVersionLimit = 7
	MaxMainlineCommitVersionLimit     = 300
)

// IsFinished returns whether or not the version has finished based on its
// status.
func (v *Version) IsFinished() bool {
	return evergreen.IsFinishedVersionStatus(v.Status)
}

// LastSuccessful returns the last successful version before the current
// version.
func (v *Version) LastSuccessful(ctx context.Context) (*Version, error) {
	lastGreen, err := VersionFindOne(ctx, VersionBySuccessfulBeforeRevision(v.Identifier, v.RevisionOrderNumber).
		Project(bson.M{VersionBuildVariantsKey: 0}).
		Sort([]string{"-" + VersionRevisionOrderNumberKey}))
	if err != nil {
		return nil, errors.Wrap(err, "retrieving last successful version")
	}
	return lastGreen, nil
}

// ActivateAndSetBuildVariants activates the version and sets its build variants.
func (v *Version) ActivateAndSetBuildVariants(ctx context.Context) error {
	bvs, err := v.GetBuildVariants(ctx)
	if err != nil {
		return errors.Wrap(err, "getting build variant info for version")
	}
	return VersionUpdateOne(
		ctx,
		bson.M{VersionIdKey: v.Id},
		bson.M{
			"$set": bson.M{
				VersionActivatedKey:     true,
				VersionBuildVariantsKey: bvs,
			},
		},
	)
}

// SetActivated sets version activated field to specified boolean.
func (v *Version) SetActivated(ctx context.Context, activated bool) error {
	if utility.FromBoolPtr(v.Activated) == activated {
		return nil
	}
	v.Activated = utility.ToBoolPtr(activated)
	return SetVersionActivated(ctx, v.Id, activated)
}

// SetVersionActivated sets version activated field to specified boolean given a version id.
func SetVersionActivated(ctx context.Context, versionId string, activated bool) error {
	return VersionUpdateOne(
		ctx,
		bson.M{VersionIdKey: versionId},
		bson.M{
			"$set": bson.M{
				VersionActivatedKey: activated,
			},
		},
	)
}

// SetAborted sets the version as aborted.
func (v *Version) SetAborted(ctx context.Context, aborted bool) error {
	v.Aborted = aborted
	return VersionUpdateOne(
		ctx,
		bson.M{VersionIdKey: v.Id},
		bson.M{
			"$set": bson.M{
				VersionAbortedKey: aborted,
			},
		},
	)
}

func (v *Version) Insert(ctx context.Context) error {
	return db.Insert(ctx, VersionCollection, v)
}

func (v *Version) IsChild() bool {
	return v.ParentPatchID != ""
}

func (v *Version) AddSatisfiedTrigger(ctx context.Context, definitionID string) error {
	if v.SatisfiedTriggers == nil {
		v.SatisfiedTriggers = []string{}
	}
	v.SatisfiedTriggers = append(v.SatisfiedTriggers, definitionID)
	return errors.Wrap(AddSatisfiedTrigger(ctx, v.Id, definitionID), "adding satisfied trigger")
}

func (v *Version) UpdateStatus(ctx context.Context, newStatus string) (modified bool, err error) {
	if v.Status == newStatus {
		return false, nil
	}

	modified, err = setVersionStatus(ctx, v.Id, newStatus)
	if err != nil {
		return false, errors.Wrapf(err, "updating status for version '%s'", v.Id)
	}

	v.Status = newStatus
	if evergreen.IsFinishedVersionStatus(newStatus) {
		v.FinishTime = time.Now()
		if modified {
			if aggErr := v.UpdateAggregateTaskCosts(ctx); aggErr != nil {
				grip.Error(errors.Wrapf(aggErr, "aggregating task costs for finished version '%s'", v.Id))
			}
		}
	}

	return modified, nil
}

func setVersionStatus(ctx context.Context, versionId, newStatus string) (modified bool, err error) {
	setFields := bson.M{VersionStatusKey: newStatus}
	if evergreen.IsFinishedVersionStatus(newStatus) {
		setFields[VersionFinishTimeKey] = time.Now()
	}
	update := bson.M{
		"$set": setFields,
	}

	res, err := evergreen.GetEnvironment().DB().Collection(VersionCollection).UpdateOne(ctx, bson.M{
		VersionIdKey:     versionId,
		VersionStatusKey: bson.M{"$ne": newStatus},
	}, update)
	if err != nil {
		return false, err
	}

	return res.ModifiedCount > 0, nil
}

// GetTimeSpent returns the total time_taken and makespan of a version for
// each task that has finished running
func (v *Version) GetTimeSpent(ctx context.Context) (time.Duration, time.Duration, error) {
	query := db.Query(task.ByVersion(v.Id)).WithFields(
		task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey)
	tasks, err := task.FindAllFirstExecution(ctx, query)
	if err != nil {
		return 0, 0, errors.Wrapf(err, "getting tasks for version '%s'", v.Id)
	}
	if tasks == nil {
		return 0, 0, errors.Errorf("no tasks found for version '%s'", v.Id)
	}

	timeTaken, makespan := task.GetTimeSpent(tasks)
	return timeTaken, makespan, nil
}

// UpdateProjectStorageMethod updates the version's parser project storage
// method.
func (v *Version) UpdateProjectStorageMethod(ctx context.Context, method evergreen.ParserProjectStorageMethod) error {
	if method == v.ProjectStorageMethod {
		return nil
	}

	if err := VersionUpdateOne(ctx, bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionProjectStorageMethodKey: method},
	}); err != nil {
		return err
	}
	v.ProjectStorageMethod = method
	return nil
}

// UpdatePreGenerationProjectStorageMethod updates the version's pre-generation parser project storage
// method.
func (v *Version) UpdatePreGenerationProjectStorageMethod(ctx context.Context, method evergreen.ParserProjectStorageMethod) error {
	if method == v.PreGenerationProjectStorageMethod {
		return nil
	}
	if err := VersionUpdateOne(ctx, bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{VersionPreGenerationProjectStorageMethodKey: method},
	}); err != nil {
		return err
	}
	v.PreGenerationProjectStorageMethod = method
	return nil
}

// GetBuildVariants returns the build variants for the version. If the version
// already has build variants cached, it'll use that; otherwise, it will load
// the build variants from the DB. If the version does not exist in the DB yet,
// it'll return v's own in-memory BuildVariants, if any.
func (v *Version) GetBuildVariants(ctx context.Context) ([]VersionBuildStatus, error) {
	if v.BuildVariants != nil {
		return v.BuildVariants, nil
	}
	versionWithBuildVariants, err := VersionFindOneIdWithBuildVariants(ctx, v.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "finding version '%s'", v.Id)
	}
	bvs := []VersionBuildStatus{}
	// If the version is nil, then the version doesn't exist in the DB, so it's
	// safe to assume that BuildVariants is not populated. This intentionally
	// does not error if the version doesn't exist in the DB so that this method
	// can be called even if the version hasn't been inserted into the DB yet.
	if versionWithBuildVariants != nil && versionWithBuildVariants.BuildVariants != nil {
		bvs = versionWithBuildVariants.BuildVariants
	}
	v.BuildVariants = bvs

	return v.BuildVariants, nil
}

// UpdateAggregateTaskCosts aggregates the actual and predicted costs from all execution tasks
// in the version and updates the version's Cost and PredictedCost fields in the database.
func (v *Version) UpdateAggregateTaskCosts(ctx context.Context) error {
	env := evergreen.GetEnvironment()
	tasksColl := env.DB().Collection(taskCollection)

	match := bson.M{
		taskVersionKey: v.Id,
		taskDisplayOnlyKey: bson.M{
			"$ne": true,
		},
	}

	pipeline := []bson.M{
		{"$match": match},
		{"$unionWith": bson.M{
			"coll": oldTaskCollection,
			"pipeline": []bson.M{
				{"$match": match},
			},
		}},
		{"$group": bson.M{
			"_id":                nil,
			"total_on_demand":    bson.M{"$sum": "$" + taskCostKey + "." + taskOnDemandCostKey},
			"total_adjusted":     bson.M{"$sum": "$" + taskCostKey + "." + taskAdjustedCostKey},
			"expected_on_demand": bson.M{"$sum": "$" + taskPredictedCostKey + "." + taskOnDemandCostKey},
			"expected_adjusted":  bson.M{"$sum": "$" + taskPredictedCostKey + "." + taskAdjustedCostKey},
		}},
	}

	cursor, err := tasksColl.Aggregate(ctx, pipeline)
	if err != nil {
		return errors.Wrap(err, "aggregating task costs for version")
	}

	var results []struct {
		TotalOnDemand     float64 `bson:"total_on_demand"`
		TotalAdjusted     float64 `bson:"total_adjusted"`
		PredictedOnDemand float64 `bson:"expected_on_demand"`
		PredictedAdjusted float64 `bson:"expected_adjusted"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return errors.Wrap(err, "reading aggregated task cost results")
	}

	var total, predicted cost.Cost
	if len(results) > 0 {
		total.OnDemandEC2Cost = results[0].TotalOnDemand
		total.AdjustedEC2Cost = results[0].TotalAdjusted
		predicted.OnDemandEC2Cost = results[0].PredictedOnDemand
		predicted.AdjustedEC2Cost = results[0].PredictedAdjusted
	}

	if err := VersionUpdateOne(ctx, bson.M{VersionIdKey: v.Id}, bson.M{
		"$set": bson.M{
			VersionCostKey:          total,
			VersionPredictedCostKey: predicted,
		},
	}); err != nil {
		return errors.Wrap(err, "updating version aggregated task costs")
	}

	v.Cost = total
	v.PredictedCost = predicted
	return nil
}

// VersionBuildStatus stores metadata relating to each build
type VersionBuildStatus struct {
	BuildVariant     string                `bson:"build_variant" json:"id"`
	DisplayName      string                `bson:"display_name,omitempty" json:"display_name,omitempty"`
	BuildId          string                `bson:"build_id,omitempty" json:"build_id,omitempty"`
	BatchTimeTasks   []BatchTimeTaskStatus `bson:"batchtime_tasks,omitempty" json:"batchtime_tasks,omitempty"`
	ActivationStatus `bson:",inline"`
}

type BatchTimeTaskStatus struct {
	TaskName         string `bson:"task_name" json:"task_name"`
	TaskId           string `bson:"task_id,omitempty" json:"task_id,omitempty"`
	ActivationStatus `bson:",inline"`
}

type ActivationStatus struct {
	Activated  bool      `bson:"activated" json:"activated"`
	ActivateAt time.Time `bson:"activate_at,omitempty" json:"activate_at,omitempty"`
}

func (s *ActivationStatus) ShouldActivate(now time.Time) bool {
	return !s.Activated && now.After(s.ActivateAt) && !utility.IsZeroTime(s.ActivateAt)
}

// VersionMetadata is used to pass information about version creation
type VersionMetadata struct {
	Revision            Revision
	TriggerID           string
	TriggerType         string
	EventID             string
	TriggerDefinitionID string
	SourceVersion       *Version
	SourceCommit        string
	IsAdHoc             bool
	Activate            bool
	User                *user.DBUser
	Message             string
	Alias               string
	PeriodicBuildID     string
	RemotePath          string
	GitTag              GitTag
	ChangedFiles        []string
}

var (
	VersionBuildStatusIdKey             = bsonutil.MustHaveTag(VersionBuildStatus{}, "BuildId")
	VersionBuildStatusDisplayNameKey    = bsonutil.MustHaveTag(VersionBuildStatus{}, "DisplayName")
	VersionBuildStatusVariantKey        = bsonutil.MustHaveTag(VersionBuildStatus{}, "BuildVariant")
	VersionBuildStatusActivatedKey      = bsonutil.MustHaveTag(VersionBuildStatus{}, "Activated")
	VersionBuildStatusBatchTimeTasksKey = bsonutil.MustHaveTag(VersionBuildStatus{}, "BatchTimeTasks")

	BatchTimeTaskStatusTaskNameKey  = bsonutil.MustHaveTag(BatchTimeTaskStatus{}, "TaskName")
	BatchTimeTaskStatusActivatedKey = bsonutil.MustHaveTag(BatchTimeTaskStatus{}, "Activated")
)

type DuplicateVersionsID struct {
	Hash      string `bson:"hash"`
	ProjectID string `bson:"project_id"`
}

type DuplicateVersions struct {
	ID       DuplicateVersionsID `bson:"_id"`
	Versions []Version           `bson:"versions"`
}

// GetMostRecentWaterfallVersion returns the most recent version, activated or unactivated, on the waterfall.
func GetMostRecentWaterfallVersion(ctx context.Context, projectId string) (*Version, error) {
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": evergreen.SystemVersionRequesterTypes,
		},
	}
	pipeline := []bson.M{
		{"$match": match},
		{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}},
		{"$limit": 1},
		{"$project": bson.M{VersionBuildVariantsKey: 0}},
	}

	res := []Version{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, errors.Errorf("could not find mainline commit for project '%s'", projectId)
	}
	return &res[0], nil
}

// GetPreviousPageCommitOrderNumber returns the first mainline commit that is LIMIT activated versions more recent than the specified commit
func GetPreviousPageCommitOrderNumber(ctx context.Context, projectId string, order int, limit int, requesters []string) (*int, error) {
	invalidRequesters, _ := utility.StringSliceSymmetricDifference(requesters, evergreen.SystemVersionRequesterTypes)
	if len(invalidRequesters) > 0 {
		return nil, errors.Errorf("invalid requesters %s", invalidRequesters)
	}
	// First check if we are already looking at the most recent commit.
	mostRecentCommit, err := GetMostRecentWaterfallVersion(ctx, projectId)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Check that the ORDER number we want to check is less the the ORDER number of the most recent commit.
	// So we don't need to check for newer commits than the most recent commit.
	if mostRecentCommit.RevisionOrderNumber <= order {
		return nil, nil
	}
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": requesters,
		},
		VersionActivatedKey:           true,
		VersionRevisionOrderNumberKey: bson.M{"$gt": order},
	}

	// We want to get the commits that are newer than the specified ORDER number, then take only the LIMIT newer activated versions then that ORDER number.
	pipeline := []bson.M{{"$match": match}, {"$sort": bson.M{VersionRevisionOrderNumberKey: 1}}, {"$limit": limit}, {"$project": bson.M{"_id": 0, VersionRevisionOrderNumberKey: 1}}}

	res := []Version{}

	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	// If there are no newer mainline commits, return nil to indicate that we are already on the first page.
	if len(res) == 0 {
		return nil, nil
	}
	// If the previous page does not contain enough active commits to populate the project health view we return 0 to indicate that the previous page has the latest commits.
	// GetMainlineCommitVersionsWithOptions returns the latest commits when 0 is passed in as the order number.
	if len(res) < limit {
		return utility.ToIntPtr(0), nil
	}

	// Return the
	return &res[len(res)-1].RevisionOrderNumber, nil
}

type MainlineCommitVersionOptions struct {
	Limit           int
	SkipOrderNumber int
	Requesters      []string
}

func GetMainlineCommitVersionsWithOptions(ctx context.Context, projectId string, opts MainlineCommitVersionOptions) ([]Version, error) {
	invalidRequesters, _ := utility.StringSliceSymmetricDifference(opts.Requesters, evergreen.SystemVersionRequesterTypes)
	if len(invalidRequesters) > 0 {
		return nil, errors.Errorf("invalid requesters %s", invalidRequesters)
	}
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey: bson.M{
			"$in": opts.Requesters,
		},
	}
	if opts.SkipOrderNumber != 0 {
		match[VersionRevisionOrderNumberKey] = bson.M{"$lt": opts.SkipOrderNumber}
	}
	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})
	limit := defaultVersionLimit
	if opts.Limit != 0 {
		limit = opts.Limit
	}

	pipeline = append(pipeline, bson.M{"$limit": limit})

	res := []Version{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(VersionCollection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating versions")
	}
	err = cursor.All(ctx, &res)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// GetVersionsOptions is a struct that holds the options for retrieving a list of versions
type GetVersionsOptions struct {
	Start          int       `json:"start"`
	RevisionEnd    int       `json:"revision_end"`
	Requester      string    `json:"requester"`
	Limit          int       `json:"limit"`
	Skip           int       `json:"skip"`
	IncludeBuilds  bool      `json:"include_builds"`
	IncludeTasks   bool      `json:"include_tasks"`
	ByBuildVariant string    `json:"by_build_variant"`
	ByTask         string    `json:"by_task"`
	CreatedAfter   time.Time `json:"created_after"`
	CreatedBefore  time.Time `json:"created_before"`
}

// GetVersionsWithOptions returns versions for a project, that satisfy a set of query parameters defined by
// the input GetVersionsOptions.
func GetVersionsWithOptions(ctx context.Context, projectName string, opts GetVersionsOptions) ([]Version, error) {
	projectId, err := GetIdForProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultVersionLimit
	}

	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey:  opts.Requester,
	}
	if opts.ByBuildVariant != "" {
		match[bsonutil.GetDottedKeyName(VersionBuildVariantsKey, VersionBuildStatusVariantKey)] = opts.ByBuildVariant
	}

	revisionFilter := bson.M{}
	if opts.Start > 0 {
		revisionFilter["$lt"] = opts.Start
		match[VersionRevisionOrderNumberKey] = revisionFilter
	}

	if opts.RevisionEnd > 0 {
		revisionFilter["$gte"] = opts.RevisionEnd
		match[VersionRevisionOrderNumberKey] = revisionFilter
	}

	if !utility.IsZeroTime(opts.CreatedAfter) && !utility.IsZeroTime(opts.CreatedBefore) {
		match[VersionCreateTimeKey] = bson.M{"$gte": opts.CreatedAfter, "$lte": opts.CreatedBefore}
	} else if !utility.IsZeroTime(opts.CreatedAfter) {
		match[VersionCreateTimeKey] = bson.M{"$gte": opts.CreatedAfter}
	} else if !utility.IsZeroTime(opts.CreatedBefore) {
		match[VersionCreateTimeKey] = bson.M{"$lte": opts.CreatedBefore}
	}

	pipeline := []bson.M{{"$match": match}}
	pipeline = append(pipeline, bson.M{"$sort": bson.M{VersionRevisionOrderNumberKey: -1}})

	// initial projection of version items
	project := bson.M{
		VersionIdentifierKey:          1,
		VersionOwnerNameKey:           1,
		VersionRepoKey:                1,
		VersionBranchKey:              1,
		VersionActivatedKey:           1,
		VersionCreateTimeKey:          1,
		VersionStartTimeKey:           1,
		VersionFinishTimeKey:          1,
		VersionRevisionKey:            1,
		VersionAuthorKey:              1,
		VersionAuthorEmailKey:         1,
		VersionMessageKey:             1,
		VersionStatusKey:              1,
		VersionBuildVariantsKey:       1,
		VersionErrorsKey:              1,
		VersionRevisionOrderNumberKey: 1,
		VersionRequesterKey:           1,
	}

	pipeline = append(pipeline, bson.M{"$project": project})
	if opts.IncludeBuilds {
		// filter builds by version and variant (if applicable)
		matchVersion := bson.M{"$expr": bson.M{"$eq": []string{"$version", "$$temp_version_id"}}}
		if opts.ByBuildVariant != "" {
			matchVersion[build.BuildVariantKey] = opts.ByBuildVariant
			matchVersion[build.ActivatedKey] = true
		}

		innerPipeline := []bson.M{{"$match": matchVersion}}

		// project out the task cache so we can rewrite it with updated data
		innerProject := bson.M{
			build.TasksKey: 0,
		}
		innerPipeline = append(innerPipeline, bson.M{"$project": innerProject})
		// include tasks and filter by task name (if applicable)
		if opts.IncludeTasks {
			taskMatch := []bson.M{
				{"$eq": []string{"$build_id", "$$temp_build_id"}},
				{"$eq": []any{"$activated", true}},
			}
			if opts.ByTask != "" {
				taskMatch = append(taskMatch, bson.M{"$eq": []string{"$display_name", opts.ByTask}})
			}
			taskLookup := bson.M{
				"from": task.Collection,
				"let":  bson.M{"temp_build_id": "$_id"},
				"as":   "tasks",
				"pipeline": []bson.M{
					{"$match": bson.M{"$expr": bson.M{"$and": taskMatch}}},
					{"$project": bson.M{"id": "$_id"}},
				},
			}
			innerPipeline = append(innerPipeline, bson.M{"$lookup": taskLookup})

			// filter out builds that don't have any tasks included
			matchTasksExist := bson.M{
				"tasks": bson.M{"$exists": true, "$ne": []any{}},
			}
			innerPipeline = append(innerPipeline, bson.M{"$match": matchTasksExist})
		}
		lookupBuilds := bson.M{
			"from":     build.Collection,
			"let":      bson.M{"temp_version_id": "$_id"},
			"as":       "build_variants",
			"pipeline": innerPipeline,
		}
		pipeline = append(pipeline, bson.M{"$lookup": lookupBuilds})
		//
		// filter out versions that don't have any activated builds
		matchBuildsExist := bson.M{
			"build_variants": bson.M{"$exists": true, "$ne": []any{}},
		}
		pipeline = append(pipeline, bson.M{"$match": matchBuildsExist})
	}

	if opts.Skip != 0 {
		pipeline = append(pipeline, bson.M{"$skip": opts.Skip})
	}
	pipeline = append(pipeline, bson.M{"$limit": opts.Limit})

	res := []Version{}

	if err := db.Aggregate(ctx, VersionCollection, pipeline, &res); err != nil {
		return nil, errors.Wrap(err, "aggregating versions and builds")
	}
	return res, nil
}

// ModifyVersionsOptions is a struct containing options necessary to modify versions.
type ModifyVersionsOptions struct {
	Priority      *int64 `json:"priority"`
	StartTimeStr  string `json:"start_time_str"`
	EndTimeStr    string `json:"end_time_str"`
	RevisionStart int    `json:"revision_start"`
	RevisionEnd   int    `json:"revision_end"`
	Requester     string `json:"requester"`
}

// GetVersionsToModify returns a slice of versions intended to be modified that satisfy the given ModifyVersionsOptions.
func GetVersionsToModify(ctx context.Context, projectName string, opts ModifyVersionsOptions, startTime, endTime time.Time) ([]Version, error) {
	projectId, err := GetIdForProject(ctx, projectName)
	if err != nil {
		return nil, err
	}
	match := bson.M{
		VersionIdentifierKey: projectId,
		VersionRequesterKey:  opts.Requester,
	}

	// setting revision numbers will take precedence over setting start and end times
	if opts.RevisionStart > 0 {
		match[VersionRevisionOrderNumberKey] = bson.M{"$lte": opts.RevisionStart, "$gte": opts.RevisionEnd}
	} else {
		match[VersionCreateTimeKey] = bson.M{"$gte": startTime, "$lte": endTime}
	}
	versions, err := VersionFind(ctx, db.Query(match))
	if err != nil {
		return nil, errors.Wrap(err, "finding versions")
	}
	return versions, nil
}

// constructManifest will construct a manifest from the given project and version.
func constructManifest(ctx context.Context, v *Version, projectRef *ProjectRef, moduleList ModuleList) (*manifest.Manifest, error) {
	if len(moduleList) == 0 {
		return nil, nil
	}
	newManifest := &manifest.Manifest{
		Id:          v.Id,
		Revision:    v.Revision,
		ProjectName: v.Identifier,
		Branch:      projectRef.Branch,
		IsBase:      v.Requester == evergreen.RepotrackerVersionRequester,
	}

	projVars, err := FindMergedProjectVars(ctx, projectRef.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "getting project vars for project '%s'", projectRef.Id)
	}
	if projVars != nil {
		expansions := util.NewExpansions(projVars.Vars)
		for i := range moduleList {
			if err = util.ExpandValues(&moduleList[i], expansions); err != nil {
				return nil, errors.Wrapf(err, "expanding module '%s'", moduleList[i].Name)
			}
		}
	}

	var baseManifest *manifest.Manifest
	shouldUseBaseRevision := utility.StringSliceContains(evergreen.PatchRequesters, v.Requester) || v.Requester == evergreen.AdHocRequester
	if shouldUseBaseRevision {
		baseManifest, err = manifest.FindFromVersion(ctx, v.Id, v.Identifier, v.Revision, v.Requester)
		if err != nil {
			return nil, errors.Wrap(err, "getting base manifest")
		}
	}

	modules := map[string]*manifest.Module{}
	for _, module := range moduleList {
		if shouldUseBaseRevision && !module.AutoUpdate && baseManifest != nil {
			if baseModule, ok := baseManifest.Modules[module.Name]; ok {
				// Reuse base if Ref is unspecified or matches base revision
				if module.Ref == "" || module.Ref == baseModule.Revision {
					modules[module.Name] = baseModule
					continue
				}
			}
		}

		mfstModule, err := getManifestModule(ctx, projectRef, module, v.Requester, v.Revision)
		if err != nil {
			return nil, errors.Wrapf(err, "module '%s'", module.Name)
		}

		modules[module.Name] = mfstModule
	}
	newManifest.Modules = modules
	return newManifest, nil
}

func getManifestModule(ctx context.Context, projectRef *ProjectRef, module Module, requester, revision string) (*manifest.Module, error) {
	owner, repo, err := module.GetOwnerAndRepo()
	if err != nil {
		return nil, errors.Wrapf(err, "getting owner and repo for '%s'", module.Name)
	}

	if module.Ref == "" {
		ghCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		revisionTime := time.Unix(0, 0)

		// If this is a mainline commit, retrieve the module's commit from the time of the mainline commit.
		// If this is a periodic build, retrieve the module's commit from the time of the periodic build.
		// Otherwise, retrieve the module's commit from the time of the patch creation.
		if !evergreen.IsPatchRequester(requester) && requester != evergreen.AdHocRequester {
			commit, err := thirdparty.GetCommitEvent(ghCtx, projectRef.Owner, projectRef.Repo, revision)
			if err != nil {
				return nil, errors.Wrapf(err, "can't get commit '%s' on '%s/%s'", revision, projectRef.Owner, projectRef.Repo)
			}
			if commit == nil || commit.Commit == nil || commit.Commit.Committer == nil {
				return nil, errors.New("malformed GitHub commit response")
			}
			revisionTime = commit.Commit.Committer.GetDate().Time
		}

		branchCommits, _, err := thirdparty.GetGithubCommits(ghCtx, owner, repo, module.Branch, revisionTime, 0)
		if err != nil {
			return nil, errors.Wrapf(err, "retrieving git branch for module '%s'", module.Name)
		}
		var sha, url string
		if len(branchCommits) > 0 {
			sha = branchCommits[0].GetSHA()
			url = branchCommits[0].GetURL()
		}

		return &manifest.Module{
			Branch:   module.Branch,
			Revision: sha,
			Repo:     repo,
			Owner:    owner,
			URL:      url,
		}, nil
	}

	ghCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	sha := module.Ref
	gitCommit, err := thirdparty.GetCommitEvent(ghCtx, owner, repo, module.Ref)
	if err != nil {
		return nil, errors.Wrapf(err, "retrieving getting git commit for module '%s' with hash '%s'", module.Name, module.Ref)
	}
	url := gitCommit.GetURL()

	return &manifest.Module{
		Branch:   module.Branch,
		Revision: sha,
		Repo:     repo,
		Owner:    owner,
		URL:      url,
	}, nil
}

// CreateManifest inserts a newly constructed manifest into the DB.
func CreateManifest(ctx context.Context, v *Version, modules ModuleList, projectRef *ProjectRef) (*manifest.Manifest, error) {
	newManifest, err := constructManifest(ctx, v, projectRef, modules)
	if err != nil {
		return nil, errors.Wrap(err, "constructing manifest")
	}
	if newManifest == nil {
		return nil, nil
	}
	_, err = newManifest.TryInsert(ctx)
	return newManifest, errors.Wrap(err, "inserting manifest")
}

type VersionsByCreateTime []Version

func (v VersionsByCreateTime) Len() int {
	return len(v)
}

func (v VersionsByCreateTime) Less(i, j int) bool {
	return v[i].CreateTime.Before(v[j].CreateTime)
}

func (v VersionsByCreateTime) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}
