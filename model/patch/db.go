package patch

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

const (
	Collection   = "patches"
	GridFSPrefix = "patchfiles"
)

// BSON fields for the patches
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
	PatchesKey              = bsonutil.MustHaveTag(Patch{}, "Patches")
	ParametersKey           = bsonutil.MustHaveTag(Patch{}, "Parameters")
	ActivatedKey            = bsonutil.MustHaveTag(Patch{}, "Activated")
	IsReconfiguredKey       = bsonutil.MustHaveTag(Patch{}, "IsReconfigured")
	ProjectStorageMethodKey = bsonutil.MustHaveTag(Patch{}, "ProjectStorageMethod")
	PatchedProjectConfigKey = bsonutil.MustHaveTag(Patch{}, "PatchedProjectConfig")
	AliasKey                = bsonutil.MustHaveTag(Patch{}, "Alias")
	githubMergeDataKey      = bsonutil.MustHaveTag(Patch{}, "GithubMergeData")
	githubPatchDataKey      = bsonutil.MustHaveTag(Patch{}, "GithubPatchData")
	MergePatchKey           = bsonutil.MustHaveTag(Patch{}, "MergePatch")
	TriggersKey             = bsonutil.MustHaveTag(Patch{}, "Triggers")
	HiddenKey               = bsonutil.MustHaveTag(Patch{}, "Hidden")

	// BSON fields for the module patch struct
	ModulePatchNameKey    = bsonutil.MustHaveTag(ModulePatch{}, "ModuleName")
	ModulePatchGithashKey = bsonutil.MustHaveTag(ModulePatch{}, "Githash")
	ModulePatchSetKey     = bsonutil.MustHaveTag(ModulePatch{}, "PatchSet")

	// BSON fields for the patch set struct
	PatchSetPatchKey   = bsonutil.MustHaveTag(PatchSet{}, "Patch")
	PatchSetSummaryKey = bsonutil.MustHaveTag(PatchSet{}, "Summary")

	// BSON fields for the patch trigger struct
	TriggerInfoAliasesKey               = bsonutil.MustHaveTag(TriggerInfo{}, "Aliases")
	TriggerInfoParentPatchKey           = bsonutil.MustHaveTag(TriggerInfo{}, "ParentPatch")
	TriggerInfoChildPatchesKey          = bsonutil.MustHaveTag(TriggerInfo{}, "ChildPatches")
	TriggerInfoDownstreamParametersKey  = bsonutil.MustHaveTag(TriggerInfo{}, "DownstreamParameters")
	TriggerInfoChildrenCompletedTimeKey = bsonutil.MustHaveTag(TriggerInfo{}, "ChildrenCompletedTime")

	// BSON fields for thirdparty.Github
	githubPatchHeadOwnerKey = bsonutil.MustHaveTag(thirdparty.GithubPatch{}, "HeadOwner")

	// BSON fields for thirdparty.GithubMergeGroup
	githubMergeGroupOrgKey                = bsonutil.MustHaveTag(thirdparty.GithubMergeGroup{}, "Org")
	githubMergeGroupRepoKey               = bsonutil.MustHaveTag(thirdparty.GithubMergeGroup{}, "Repo")
	githubMergeGroupHeadSHAKey            = bsonutil.MustHaveTag(thirdparty.GithubMergeGroup{}, "HeadSHA")
	githubMergeGroupRemovedFromQueueAtKey = bsonutil.MustHaveTag(thirdparty.GithubMergeGroup{}, "RemovedFromQueueAt")
	githubMergeGroupRemovalReasonKey      = bsonutil.MustHaveTag(thirdparty.GithubMergeGroup{}, "RemovalReason")
)

// ProjectCreateTimeIndex is a partial index used to speed up finding GitHub Merge Queue patches
var (
	ProjectCreateTimeIndex = bson.D{
		{Key: ProjectKey, Value: 1},
		{Key: CreateTimeKey, Value: -1},
	}
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

func ByGithash(githash string) db.Q {
	return db.Query(bson.M{bsonutil.GetDottedKeyName(githubPatchDataKey, headHashKey): githash})
}

type ProjectOrUserPatchesOptions struct {
	Author         *string
	CountLimit     int
	IncludeHidden  *bool
	Limit          int
	OnlyMergeQueue *bool
	Page           int
	PatchName      string
	Project        *string
	Requesters     []string
	Statuses       []string
}

// Based off of the implementation for Patch.GetRequester.
var requesterExpression = bson.M{
	"$switch": bson.M{
		"branches": []bson.M{
			// Should match implementation of IsGithubPRPatch().
			{
				"case": bson.M{
					"$and": []bson.M{
						{"$ifNull": []any{"$" + githubPatchDataKey, false}},
						{"$ne": []string{"$" + bsonutil.GetDottedKeyName(githubPatchDataKey, githubPatchHeadOwnerKey), ""}},
					},
				},
				"then": evergreen.GithubPRRequester,
			},
			// Should match implementation of IsMergeQueuePatch().
			{
				"case": bson.M{
					"$or": []bson.M{
						{"$and": []bson.M{
							{"$ifNull": []any{"$" + githubMergeDataKey, false}},
							{"$ne": []string{"$" + bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupHeadSHAKey), ""}},
						}},
						{"$eq": []string{"$" + AliasKey, evergreen.CommitQueueAlias}},
					},
				},
				"then": evergreen.GithubMergeRequester,
			},
		},
		"default": evergreen.PatchVersionRequester,
	},
}

// buildPatchFilterPipeline constructs the common filtering pipeline for patch queries.
// Returns the pipeline stages and a boolean indicating if we're filtering for merge queue only.
// If includeSort is false, the sort stage is omitted (useful for count queries where order doesn't matter).
func buildPatchFilterPipeline(opts ProjectOrUserPatchesOptions, includeSort bool) ([]bson.M, bool) {
	pipeline := []bson.M{}
	match := bson.M{}

	if !utility.FromBoolTPtr(opts.IncludeHidden) {
		match[HiddenKey] = bson.M{"$ne": true}
	}

	if opts.PatchName != "" {
		match[DescriptionKey] = bson.M{"$regex": opts.PatchName, "$options": "i"}
	}

	if len(opts.Statuses) > 0 {
		match[StatusKey] = bson.M{"$in": opts.Statuses}
	}
	if opts.Author != nil {
		match[AuthorKey] = utility.FromStringPtr(opts.Author)
	}
	if opts.Project != nil {
		match[ProjectKey] = utility.FromStringPtr(opts.Project)
	}

	// Validate requesters and check if we're only filtering for merge queue patches.
	patchRequesters := []string{}
	for _, requester := range opts.Requesters {
		if evergreen.IsPatchRequester(requester) {
			patchRequesters = append(patchRequesters, requester)
		}
	}
	onlyMergeQueue := len(patchRequesters) == 1 && patchRequesters[0] == evergreen.GithubMergeRequester

	// This filter matches the logic in IsMergeQueuePatch() and results in significantly fewer documents being retrieved from the db.
	if onlyMergeQueue {
		match["$or"] = []bson.M{
			{
				bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupHeadSHAKey): bson.M{
					"$exists": true,
					"$ne":     "",
				},
			},
			{AliasKey: evergreen.CommitQueueAlias},
		}
	}

	pipeline = append(pipeline, bson.M{"$match": match})

	if includeSort {
		sortStage := bson.M{
			"$sort": bson.M{
				CreateTimeKey: -1,
			},
		}
		pipeline = append(pipeline, sortStage)
	}

	// Apply requester filtering using the computed requester expression.
	// Skip this step if we're only filtering for merge queue patches since we already applied the optimization above.
	if len(patchRequesters) > 0 && !onlyMergeQueue {
		pipeline = append(pipeline, bson.M{"$addFields": bson.M{"requester": requesterExpression}})
		pipeline = append(pipeline, bson.M{"$match": bson.M{"requester": bson.M{"$in": patchRequesters}}})
	}

	return pipeline, onlyMergeQueue
}

// ProjectOrUserPatchesPage returns a page of patches matching the filter criteria.
func ProjectOrUserPatchesPage(ctx context.Context, opts ProjectOrUserPatchesOptions) ([]Patch, error) {
	ctx = utility.ContextWithAppendedAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "ProjectOrUserPatchesPage")})

	if opts.Project != nil && opts.Author != nil {
		return nil, errors.New("can't set both project and author")
	}

	pipeline, onlyMergeQueue := buildPatchFilterPipeline(opts, true)

	// Exclude large patch diff data to avoid exceeding MongoDB's 16MB document limit.
	pipeline = append(pipeline, bson.M{"$project": ExcludePatchDiff})

	if opts.Page > 0 {
		pipeline = append(pipeline, bson.M{"$skip": opts.Page * opts.Limit})
	}
	if opts.Limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": opts.Limit})
	}

	env := evergreen.GetEnvironment()

	var aggregateOpts *options.AggregateOptions
	if onlyMergeQueue {
		aggregateOpts = options.Aggregate().SetHint(ProjectCreateTimeIndex)
	}

	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline, aggregateOpts)
	if err != nil {
		return nil, err
	}

	results := []Patch{}
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// ProjectOrUserPatchesCount returns the count of patches matching the filter criteria.
// An upper threshold is set since the precise document count doesn't really matter.
func ProjectOrUserPatchesCount(ctx context.Context, opts ProjectOrUserPatchesOptions) (int, error) {
	ctx = utility.ContextWithAppendedAttributes(ctx, []attribute.KeyValue{attribute.String(evergreen.AggregationNameOtelAttribute, "ProjectOrUserPatchesCount")})

	if opts.Project != nil && opts.Author != nil {
		return 0, errors.New("can't set both project and author")
	}

	if opts.CountLimit <= 0 || opts.CountLimit > 10000 {
		opts.CountLimit = 10000
	}

	pipeline, onlyMergeQueue := buildPatchFilterPipeline(opts, false)

	// For performance, use $limit instead of $count to avoid scanning all matching documents.
	pipeline = append(pipeline, bson.M{"$limit": opts.CountLimit})
	pipeline = append(pipeline, bson.M{"$count": "count"})

	env := evergreen.GetEnvironment()

	var aggregateOpts *options.AggregateOptions
	if onlyMergeQueue {
		aggregateOpts = options.Aggregate().SetHint(ProjectCreateTimeIndex)
	}

	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline, aggregateOpts)
	if err != nil {
		return 0, err
	}

	type countResult struct {
		Count int `bson:"count"`
	}
	countResults := []countResult{}
	if err = cursor.All(ctx, &countResults); err != nil {
		return 0, err
	}

	if len(countResults) == 0 {
		return 0, nil
	}

	if countResults[0].Count == opts.CountLimit {
		// Indicate that we've maxed out at our count limit
		return math.MaxInt32, nil
	}

	return countResults[0].Count, nil
}

// ByUserPaginated produces a query that returns patches by the given user
// before/after the input time, sorted by creation time and limited
func ByUserPaginated(user string, ts time.Time, limit int) db.Q {
	return db.Query(bson.M{
		AuthorKey:     user,
		CreateTimeKey: bson.M{"$lte": ts},
	}).Sort([]string{"-" + CreateTimeKey}).Limit(limit)
}

func byUser(user string) bson.M {
	return bson.M{AuthorKey: user}
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
func FindOne(ctx context.Context, query db.Q) (*Patch, error) {
	patch := &Patch{}
	err := db.FindOneQ(ctx, Collection, query, patch)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return patch, err
}

func FindOneId(ctx context.Context, id string) (*Patch, error) {
	if !IsValidId(id) {
		return nil, errors.Errorf("'%s' is not a valid ObjectId", id)
	}
	return FindOne(ctx, ByStringId(id))
}

// Find runs a patch query, returning all patches that satisfy the query.
func Find(ctx context.Context, query db.Q) ([]Patch, error) {
	patches := []Patch{}
	err := db.FindAllQ(ctx, Collection, query, &patches)
	return patches, err
}

// Remove removes all patch documents that satisfy the query.
func Remove(ctx context.Context, query db.Q) error {
	return db.RemoveAllQ(ctx, Collection, query)
}

// UpdateAll runs an update on all patch documents.
func UpdateAll(ctx context.Context, query any, update any) (info *adb.ChangeInfo, err error) {
	return db.UpdateAll(ctx, Collection, query, update)
}

// UpdateOne runs an update on a single patch document.
func UpdateOne(ctx context.Context, query any, update any) error {
	return db.Update(ctx, Collection, query, update)
}

// PatchesByProject builds a query for patches that match the given
// project's id.
func PatchesByProject(projectId string, ts time.Time, limit int) db.Q {
	return db.Query(bson.M{
		CreateTimeKey: bson.M{"$lte": ts},
		ProjectKey:    projectId,
	}).Sort([]string{"-" + CreateTimeKey}).Limit(limit)
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

// ConsolidatePatchesForUser updates all patches authored by oldAuthor to be authored by newAuthor,
// and if any patches have been authored by the new author already, update the patch numbers to come after the new author.
func ConsolidatePatchesForUser(ctx context.Context, oldAuthor string, newUsr *user.DBUser) error {

	// It's not likely that the user would've already created patches for the new user, but if there are any, make
	// sure that they don't have overlapping patch numbers.
	patchesForNewAuthor, err := Find(ctx, db.Query(byUser(newUsr.Id)))
	if err != nil {
		return errors.Wrapf(err, "finding existing patches for '%s'", newUsr.Id)
	}
	if len(patchesForNewAuthor) > 0 {
		for _, p := range patchesForNewAuthor {
			patchNum, err := newUsr.IncPatchNumber(ctx)
			if err != nil {
				return errors.Wrap(err, "incrementing patch number to resolve existing patches")
			}
			update := bson.M{"$set": bson.M{NumberKey: patchNum}}
			if err := UpdateOne(ctx, bson.M{IdKey: p.Id}, update); err != nil {
				return errors.Wrap(err, "updating patch number")
			}
		}
	}

	// Move all patches from the old author over to the new one.
	update := bson.M{
		"$set": bson.M{AuthorKey: newUsr.Id},
	}
	_, err = UpdateAll(ctx, byUser(oldAuthor), update)
	return err
}

// FindLatestGithubPRPatch returns the latest PR patch for the given PR, if there is one.
func FindLatestGithubPRPatch(ctx context.Context, owner, repo string, prNumber int) (*Patch, error) {
	patches, err := Find(ctx, db.Query(bson.M{
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

func FindProjectForPatch(ctx context.Context, patchID mgobson.ObjectId) (string, error) {
	p, err := FindOne(ctx, ById(patchID).Project(bson.M{ProjectKey: 1}))
	if err != nil {
		return "", err
	}
	if p == nil {
		return "", errors.New("patch not found")
	}
	return p.Project, nil
}

// GetFinalizedChildPatchIdsForPatch returns patchIds for any finalized children of the given patch.
func GetFinalizedChildPatchIdsForPatch(ctx context.Context, patchID string) ([]string, error) {
	withKey := bsonutil.GetDottedKeyName(TriggersKey, TriggerInfoChildPatchesKey)
	//do the same for child patches
	p, err := FindOne(ctx, ByStringId(patchID).WithFields(withKey))
	if err != nil {
		return nil, errors.Wrapf(err, "finding patch '%s'", patchID)
	}
	if p == nil {
		return nil, errors.Wrapf(err, "patch '%s' not found", patchID)
	}
	if !p.IsParent() {
		return nil, nil
	}

	childPatches, err := Find(ctx, ByStringIds(p.Triggers.ChildPatches).WithFields(VersionKey))
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

// FindMergeQueuePatchesByProject returns all active merge queue patches for a project
func FindMergeQueuePatchesByProject(ctx context.Context, projectID string) ([]Patch, error) {
	timeThreshold := time.Now().Add(-24 * time.Hour)

	query := bson.M{
		ProjectKey: projectID,
		AliasKey:   evergreen.CommitQueueAlias,
		StatusKey: bson.M{
			"$nin": []string{evergreen.VersionFailed, evergreen.VersionSucceeded},
		},
		CreateTimeKey: bson.M{
			"$gte": timeThreshold,
		},
		bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupRemovedFromQueueAtKey): bson.M{
			"$exists": false,
		},
	}

	return Find(ctx, db.Query(query))
}

// MarkMergeQueuePatchesRemovedFromQueue updates patches matching the given HeadSHA to mark them
// as removed from the GitHub merge queue. Returns the number of patches updated.
func MarkMergeQueuePatchesRemovedFromQueue(ctx context.Context, org, repo, headSHA, reason string) (int, error) {
	if headSHA == "" {
		return 0, errors.New("headSHA cannot be empty")
	}
	if reason == "" {
		return 0, errors.New("reason cannot be empty")
	}

	query := bson.M{
		bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupOrgKey):     org,
		bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupRepoKey):    repo,
		bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupHeadSHAKey): headSHA,
		bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupRemovedFromQueueAtKey): bson.M{
			"$exists": false,
		},
	}

	update := bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupRemovedFromQueueAtKey): time.Now().UTC().Round(time.Millisecond),
			bsonutil.GetDottedKeyName(githubMergeDataKey, githubMergeGroupRemovalReasonKey):      reason,
		},
	}

	info, err := UpdateAll(ctx, query, update)
	if err != nil {
		return 0, errors.Wrap(err, "updating patches")
	}

	return info.Updated, nil
}
