package patch

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	Collection   = "patches"
	GridFSPrefix = "patchfiles"
)

// BSON fields for the patches
//
//nolint:megacheck,unused
var (
	IdKey                   = bsonutil.MustHaveTag(Patch{}, "Id")
	DescriptionKey          = bsonutil.MustHaveTag(Patch{}, "Description")
	ProjectKey              = bsonutil.MustHaveTag(Patch{}, "Project")
	GithashKey              = bsonutil.MustHaveTag(Patch{}, "Githash")
	AuthorKey               = bsonutil.MustHaveTag(Patch{}, "Author")
	NumberKey               = bsonutil.MustHaveTag(Patch{}, "PatchNumber")
	VersionKey              = bsonutil.MustHaveTag(Patch{}, "Version")
	StatusKey               = bsonutil.MustHaveTag(Patch{}, "Status")
	CreateTimeKey           = bsonutil.MustHaveTag(Patch{}, "CreateTime")
	StartTimeKey            = bsonutil.MustHaveTag(Patch{}, "StartTime")
	FinishTimeKey           = bsonutil.MustHaveTag(Patch{}, "FinishTime")
	BuildVariantsKey        = bsonutil.MustHaveTag(Patch{}, "BuildVariants")
	TasksKey                = bsonutil.MustHaveTag(Patch{}, "Tasks")
	VariantsTasksKey        = bsonutil.MustHaveTag(Patch{}, "VariantsTasks")
	SyncAtEndOptionsKey     = bsonutil.MustHaveTag(Patch{}, "SyncAtEndOpts")
	PatchesKey              = bsonutil.MustHaveTag(Patch{}, "Patches")
	ParametersKey           = bsonutil.MustHaveTag(Patch{}, "Parameters")
	ActivatedKey            = bsonutil.MustHaveTag(Patch{}, "Activated")
	ProjectStorageMethodKey = bsonutil.MustHaveTag(Patch{}, "ProjectStorageMethod")
	PatchedProjectConfigKey = bsonutil.MustHaveTag(Patch{}, "PatchedProjectConfig")
	AliasKey                = bsonutil.MustHaveTag(Patch{}, "Alias")
	githubPatchDataKey      = bsonutil.MustHaveTag(Patch{}, "GithubPatchData")
	MergePatchKey           = bsonutil.MustHaveTag(Patch{}, "MergePatch")
	TriggersKey             = bsonutil.MustHaveTag(Patch{}, "Triggers")
	HiddenKey               = bsonutil.MustHaveTag(Patch{}, "Hidden")

	// BSON fields for sync at end struct
	SyncAtEndOptionsBuildVariantsKey = bsonutil.MustHaveTag(SyncAtEndOptions{}, "BuildVariants")
	SyncAtEndOptionsTasksKey         = bsonutil.MustHaveTag(SyncAtEndOptions{}, "Tasks")
	SyncAtEndOptionsVariantsTasksKey = bsonutil.MustHaveTag(SyncAtEndOptions{}, "VariantsTasks")
	SyncAtEndOptionsStatusesKey      = bsonutil.MustHaveTag(SyncAtEndOptions{}, "Statuses")
	SyncAtEndOptionsTimeoutKey       = bsonutil.MustHaveTag(SyncAtEndOptions{}, "Timeout")

	// BSON fields for the module patch struct
	ModulePatchNameKey    = bsonutil.MustHaveTag(ModulePatch{}, "ModuleName")
	ModulePatchGithashKey = bsonutil.MustHaveTag(ModulePatch{}, "Githash")
	ModulePatchSetKey     = bsonutil.MustHaveTag(ModulePatch{}, "PatchSet")

	// BSON fields for the patch set struct
	PatchSetPatchKey   = bsonutil.MustHaveTag(PatchSet{}, "Patch")
	PatchSetSummaryKey = bsonutil.MustHaveTag(PatchSet{}, "Summary")

	// BSON fields for the patch trigger struct
	TriggerInfoAliasesKey              = bsonutil.MustHaveTag(TriggerInfo{}, "Aliases")
	TriggerInfoParentPatchKey          = bsonutil.MustHaveTag(TriggerInfo{}, "ParentPatch")
	TriggerInfoChildPatchesKey         = bsonutil.MustHaveTag(TriggerInfo{}, "ChildPatches")
	TriggerInfoDownstreamParametersKey = bsonutil.MustHaveTag(TriggerInfo{}, "DownstreamParameters")
)

// Query Validation

// IsValidId returns whether the supplied Id is a valid patch doc id (BSON ObjectId).
func IsValidId(id string) bool {
	return mgobson.IsObjectIdHex(id)
}

// NewId constructs a valid patch Id from the given hex string.
func NewId(id string) mgobson.ObjectId { return mgobson.ObjectIdHex(id) }

// Queries

// ById produces a query to return the patch with the given _id.
func ById(id mgobson.ObjectId) db.Q {
	return db.Query(bson.M{IdKey: id})
}

func ByStringId(id string) db.Q {
	return db.Query(bson.M{IdKey: NewId(id)})
}

func ByStringIds(ids []string) db.Q {
	objectIds := []mgobson.ObjectId{}
	for _, id := range ids {
		if IsValidId(id) {
			objectIds = append(objectIds, NewId(id))
		} else {
			grip.Debug(message.Fields{
				"message": "patch id is not valid",
				"id":      id,
			})
		}
	}
	return db.Query(bson.M{IdKey: bson.M{"$in": objectIds}})
}

var commitQueueFilter = bson.M{"$ne": evergreen.CommitQueueAlias}

// ByProject produces a query that returns projects with the given identifier.
func ByProjectAndCommitQueue(project string, filterCommitQueue bool) db.Q {
	q := bson.M{ProjectKey: project}
	if filterCommitQueue {
		q[AliasKey] = commitQueueFilter
	}
	return db.Query(q)
}

// ByUser produces a query that returns patches by the given user.
func ByUserAndCommitQueue(user string, filterCommitQueue bool) db.Q {
	q := bson.M{AuthorKey: user}
	if filterCommitQueue {
		q[AliasKey] = commitQueueFilter
	}

	return db.Query(q)
}

type ByPatchNameStatusesCommitQueuePaginatedOptions struct {
	Author             *string
	Project            *string
	PatchName          string
	Statuses           []string
	Page               int
	Limit              int
	IncludeCommitQueue *bool
	IncludeHidden      *bool
	OnlyCommitQueue    *bool
}

func ByPatchNameStatusesCommitQueuePaginated(ctx context.Context, opts ByPatchNameStatusesCommitQueuePaginatedOptions) ([]Patch, int, error) {
	if opts.OnlyCommitQueue != nil && opts.IncludeCommitQueue != nil {
		return nil, 0, errors.New("can't both include commit queue patches and also set only including commit queue patches")
	}
	if opts.Project != nil && opts.Author != nil {
		return nil, 0, errors.New("can't set both project and author")
	}
	pipeline := []bson.M{}
	match := bson.M{}
	// Conditionally add the commit queue filter if the user is explicitly filtering on it.
	// This is only used on the project patches page when we want to conditionally only show the commit queue patches.
	if utility.FromBoolPtr(opts.OnlyCommitQueue) {
		match[AliasKey] = evergreen.CommitQueueAlias
	}

	// This is only used on the user patches page when we want to filter out the commit queue
	if opts.IncludeCommitQueue != nil && !utility.FromBoolPtr(opts.IncludeCommitQueue) {
		match[AliasKey] = commitQueueFilter
	}

	if !utility.FromBoolTPtr(opts.IncludeHidden) {
		match[HiddenKey] = bson.M{"$ne": true}
	}

	if opts.PatchName != "" {
		match[DescriptionKey] = bson.M{"$regex": opts.PatchName, "$options": "i"}
	}
	if len(opts.Statuses) > 0 {
		// Verify that we're considering the legacy patch status as well; we'll remove this logic in EVG-20032.
		if len(utility.StringSliceIntersection(opts.Statuses, evergreen.VersionSucceededStatuses)) > 0 {
			opts.Statuses = utility.UniqueStrings(append(opts.Statuses, evergreen.VersionSucceededStatuses...))
		}
		match[StatusKey] = bson.M{"$in": opts.Statuses}
	}
	if opts.Author != nil {
		match[AuthorKey] = utility.FromStringPtr(opts.Author)
	}
	if opts.Project != nil {
		match[ProjectKey] = utility.FromStringPtr(opts.Project)
	}
	pipeline = append(pipeline, bson.M{"$match": match})

	sort := bson.M{
		"$sort": bson.M{
			CreateTimeKey: -1,
		},
	}
	// paginatePipeline will be used for the results
	paginatePipeline := append(pipeline, sort)
	if opts.Page > 0 {
		skipStage := bson.M{
			"$skip": opts.Page * opts.Limit,
		}
		paginatePipeline = append(paginatePipeline, skipStage)
	}
	if opts.Limit > 0 {
		limitStage := bson.M{
			"$limit": opts.Limit,
		}
		paginatePipeline = append(paginatePipeline, limitStage)
	}

	// Will be used to get the total count of the filtered patches
	countPipeline := append(pipeline, bson.M{"$count": "count"})

	results := []Patch{}
	env := evergreen.GetEnvironment()
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, paginatePipeline)
	if err != nil {
		return nil, 0, err
	}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}

	type countResult struct {
		Count int `bson:"count"`
	}
	countResults := []countResult{}
	cursor, err = env.DB().Collection(Collection).Aggregate(ctx, countPipeline)
	if err != nil {
		return nil, 0, err
	}
	if err = cursor.All(ctx, &countResults); err != nil {
		return nil, 0, err
	}
	if len(countResults) == 0 {
		return results, 0, nil
	}
	return results, countResults[0].Count, nil
}

// ByUserPaginated produces a query that returns patches by the given user
// before/after the input time, sorted by creation time and limited
func ByUserPaginated(user string, ts time.Time, limit int) db.Q {
	return db.Query(bson.M{
		AuthorKey:     user,
		CreateTimeKey: bson.M{"$lte": ts},
	}).Sort([]string{"-" + CreateTimeKey}).Limit(limit)
}

// MostRecentPatchByUserAndProject returns the latest patch made by the user for the project.
func MostRecentPatchByUserAndProject(user, project string) db.Q {
	return db.Query(bson.M{
		AuthorKey:    user,
		ProjectKey:   project,
		ActivatedKey: true,
		AliasKey:     bson.M{"$nin": []string{evergreen.GithubPRAlias, evergreen.CommitQueueAlias}},
	}).Sort([]string{"-" + CreateTimeKey}).Limit(1)
}

// ByVersion produces a query that returns the patch for a given version.
func ByVersion(version string) db.Q {
	return db.Query(bson.M{VersionKey: version})
}

// ByVersion produces a query that returns the patch for a given version.
func ByVersions(versions []string) db.Q {
	return db.Query(bson.M{VersionKey: bson.M{"$in": versions}})
}

// ExcludePatchDiff is a projection that excludes diff data, helping load times.
var ExcludePatchDiff = bson.M{
	bsonutil.GetDottedKeyName(PatchesKey, ModulePatchSetKey, PatchSetPatchKey): 0,
}

// Query Functions

// FindOne runs a patch query, returning one patch.
func FindOne(query db.Q) (*Patch, error) {
	patch := &Patch{}
	err := db.FindOneQ(Collection, query, patch)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return patch, err
}

func FindOneId(id string) (*Patch, error) {
	if !IsValidId(id) {
		return nil, errors.Errorf("'%s' is not a valid ObjectId", id)
	}
	return FindOne(ByStringId(id))
}

// Find runs a patch query, returning all patches that satisfy the query.
func Find(query db.Q) ([]Patch, error) {
	patches := []Patch{}
	err := db.FindAllQ(Collection, query, &patches)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return patches, err
}

// Count returns the number of patches that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}

// Remove removes all patch documents that satisfy the query.
func Remove(query db.Q) error {
	return db.RemoveAllQ(Collection, query)
}

// UpdateAll runs an update on all patch documents.
func UpdateAll(query interface{}, update interface{}) (info *adb.ChangeInfo, err error) {
	return db.UpdateAll(Collection, query, update)
}

// UpdateOne runs an update on a single patch document.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(Collection, query, update)
}

// PatchesByProject builds a query for patches that match the given
// project's id.
func PatchesByProject(projectId string, ts time.Time, limit int) db.Q {
	return db.Query(bson.M{
		CreateTimeKey: bson.M{"$lte": ts},
		ProjectKey:    projectId,
	}).Sort([]string{"-" + CreateTimeKey}).Limit(limit)
}

// FindFailedCommitQueuePatchesInTimeRange returns failed patches if they started or failed within range.
func FindFailedCommitQueuePatchesInTimeRange(projectID string, startTime, endTime time.Time) ([]Patch, error) {
	query := bson.M{
		ProjectKey: projectID,
		StatusKey:  evergreen.VersionFailed,
		AliasKey:   evergreen.CommitQueueAlias,
		"$or": []bson.M{
			{"$and": []bson.M{
				{StartTimeKey: bson.M{"$lte": endTime}},
				{StartTimeKey: bson.M{"$gte": startTime}},
			}},
			{"$and": []bson.M{
				{FinishTimeKey: bson.M{"$lte": endTime}},
				{FinishTimeKey: bson.M{"$gte": startTime}},
			}},
		},
	}
	return Find(db.Query(query).Sort([]string{CreateTimeKey}))
}

// ByGithubPRAndCreatedBefore finds all patches that were created for a GitHub
// PR before the given timestamp.
func ByGithubPRAndCreatedBefore(t time.Time, owner, repo string, prNumber int) db.Q {
	return db.Query(bson.M{
		CreateTimeKey: bson.M{
			"$lt": t,
		},
		bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.GithubPatchBaseOwnerKey): owner,
		bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.GithubPatchBaseRepoKey):  repo,
		bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.GithubPatchPRNumberKey):  prNumber,
	})
}

// FindLatestGithubPRPatch returns the latest PR patch for the given PR, if there is one.
func FindLatestGithubPRPatch(owner, repo string, prNumber int) (*Patch, error) {
	patches, err := Find(db.Query(bson.M{
		AliasKey: bson.M{"$ne": evergreen.CommitQueueAlias},
		bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.GithubPatchBaseOwnerKey): owner,
		bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.GithubPatchBaseRepoKey):  repo,
		bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.GithubPatchPRNumberKey):  prNumber,
	}).Sort([]string{"-" + CreateTimeKey}).Limit(1))
	if err != nil {
		return nil, err
	}
	if len(patches) == 0 {
		return nil, nil
	}
	return &patches[0], nil
}

func FindProjectForPatch(patchID mgobson.ObjectId) (string, error) {
	p, err := FindOne(ById(patchID).Project(bson.M{ProjectKey: 1}))
	if err != nil {
		return "", err
	}
	if p == nil {
		return "", errors.New("patch not found")
	}
	return p.Project, nil
}

// GetFinalizedChildPatchIdsForPatch returns patchIds for any finalized children of the given patch.
func GetFinalizedChildPatchIdsForPatch(patchID string) ([]string, error) {
	withKey := bsonutil.GetDottedKeyName(TriggersKey, TriggerInfoChildPatchesKey)
	//do the same for child patches
	p, err := FindOne(ByStringId(patchID).WithFields(withKey))
	if err != nil {
		return nil, errors.Wrapf(err, "finding patch '%s'", patchID)
	}
	if p == nil {
		return nil, errors.Wrapf(err, "patch '%s' not found", patchID)
	}
	if !p.IsParent() {
		return nil, nil
	}

	childPatches, err := Find(ByStringIds(p.Triggers.ChildPatches).WithFields(VersionKey))
	if err != nil {
		return nil, errors.Wrap(err, "getting child patches")
	}
	res := []string{}
	for _, child := range childPatches {
		if child.Version != "" {
			res = append(res, child.Id.Hex())
		}
	}
	return res, nil
}
