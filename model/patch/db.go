package patch

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	Collection   = "patches"
	GridFSPrefix = "patchfiles"
)

// BSON fields for the patches
//nolint: deadcode, megacheck, unused
var (
	IdKey               = bsonutil.MustHaveTag(Patch{}, "Id")
	DescriptionKey      = bsonutil.MustHaveTag(Patch{}, "Description")
	ProjectKey          = bsonutil.MustHaveTag(Patch{}, "Project")
	GithashKey          = bsonutil.MustHaveTag(Patch{}, "Githash")
	AuthorKey           = bsonutil.MustHaveTag(Patch{}, "Author")
	NumberKey           = bsonutil.MustHaveTag(Patch{}, "PatchNumber")
	VersionKey          = bsonutil.MustHaveTag(Patch{}, "Version")
	StatusKey           = bsonutil.MustHaveTag(Patch{}, "Status")
	CreateTimeKey       = bsonutil.MustHaveTag(Patch{}, "CreateTime")
	StartTimeKey        = bsonutil.MustHaveTag(Patch{}, "StartTime")
	FinishTimeKey       = bsonutil.MustHaveTag(Patch{}, "FinishTime")
	BuildVariantsKey    = bsonutil.MustHaveTag(Patch{}, "BuildVariants")
	TasksKey            = bsonutil.MustHaveTag(Patch{}, "Tasks")
	VariantsTasksKey    = bsonutil.MustHaveTag(Patch{}, "VariantsTasks")
	SyncAtEndOptionsKey = bsonutil.MustHaveTag(Patch{}, "SyncAtEndOpts")
	PatchesKey          = bsonutil.MustHaveTag(Patch{}, "Patches")
	ParametersKey       = bsonutil.MustHaveTag(Patch{}, "Parameters")
	ActivatedKey        = bsonutil.MustHaveTag(Patch{}, "Activated")
	PatchedConfigKey    = bsonutil.MustHaveTag(Patch{}, "PatchedConfig")
	AliasKey            = bsonutil.MustHaveTag(Patch{}, "Alias")
	githubPatchDataKey  = bsonutil.MustHaveTag(Patch{}, "GithubPatchData")
	MergePatchKey       = bsonutil.MustHaveTag(Patch{}, "MergePatch")

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

func ByIds(ids []mgobson.ObjectId) db.Q {
	return db.Query(bson.M{IdKey: bson.M{"$in": ids}})
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

func ByUserPatchNameStatusesCommitQueuePaginated(user, patchName string, statuses []string, includeCommitQueue bool, page int, limit int) db.Q {
	queryInterface := bson.M{
		AuthorKey: user,
	}
	if patchName != "" {
		queryInterface[DescriptionKey] = bson.M{"$regex": patchName, "$options": "i"}
	}
	if len(statuses) > 0 {
		queryInterface[StatusKey] = bson.M{"$in": statuses}
	}
	if includeCommitQueue == false {
		queryInterface[AliasKey] = commitQueueFilter
	}
	q := db.Query(queryInterface).Sort([]string{"-" + CreateTimeKey})
	if page > 0 {
		q = q.Skip(page * limit)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	return q
}

// ByUserPaginated produces a query that returns patches by the given user
// before/after the input time, sorted by creation time and limited
func ByUserPaginated(user string, ts time.Time, limit int) db.Q {
	return db.Query(bson.M{
		AuthorKey:     user,
		CreateTimeKey: bson.M{"$lte": ts},
	}).Sort([]string{"-" + CreateTimeKey}).Limit(limit)
}

// ByUserProjectAndGitspec produces a query that returns patches by the given
// patch author, project, and gitspec.
func ByUserProjectAndGitspec(user string, project string, gitspec string) db.Q {
	return db.Query(bson.M{
		AuthorKey:  user,
		ProjectKey: project,
		GithashKey: gitspec,
	})
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

// FindFailedCommitQueuePatchesInTimeRange returns failed patches if they started within range,
// or if they were never started but finished within time range. (i.e. timed out)
func FindFailedCommitQueuePatchesinTimeRange(projectID string, startTime, endTime time.Time) ([]Patch, error) {
	query := bson.M{
		ProjectKey: projectID,
		StatusKey:  evergreen.PatchFailed,
		AliasKey:   evergreen.CommitQueueAlias,
		"$or": []bson.M{
			{"$and": []bson.M{
				{StartTimeKey: bson.M{"$lte": endTime}},
				{StartTimeKey: bson.M{"$gte": startTime}},
			}},
			{"$and": []bson.M{
				{StartTimeKey: time.Time{}},
				{FinishTimeKey: bson.M{"$lte": endTime}},
				{FinishTimeKey: bson.M{"$gte": startTime}},
			}},
		},
	}
	return Find(db.Query(query).Sort([]string{CreateTimeKey}))
}

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
