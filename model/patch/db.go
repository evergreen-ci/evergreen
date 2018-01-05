package patch

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection   = "patches"
	GridFSPrefix = "patchfiles"
)

// BSON fields for the patches
//nolint: deadcode, megacheck
var (
	IdKey              = bsonutil.MustHaveTag(Patch{}, "Id")
	DescriptionKey     = bsonutil.MustHaveTag(Patch{}, "Description")
	ProjectKey         = bsonutil.MustHaveTag(Patch{}, "Project")
	GithashKey         = bsonutil.MustHaveTag(Patch{}, "Githash")
	AuthorKey          = bsonutil.MustHaveTag(Patch{}, "Author")
	NumberKey          = bsonutil.MustHaveTag(Patch{}, "PatchNumber")
	VersionKey         = bsonutil.MustHaveTag(Patch{}, "Version")
	StatusKey          = bsonutil.MustHaveTag(Patch{}, "Status")
	CreateTimeKey      = bsonutil.MustHaveTag(Patch{}, "CreateTime")
	StartTimeKey       = bsonutil.MustHaveTag(Patch{}, "StartTime")
	FinishTimeKey      = bsonutil.MustHaveTag(Patch{}, "FinishTime")
	BuildVariantsKey   = bsonutil.MustHaveTag(Patch{}, "BuildVariants")
	TasksKey           = bsonutil.MustHaveTag(Patch{}, "Tasks")
	VariantsTasksKey   = bsonutil.MustHaveTag(Patch{}, "VariantsTasks")
	PatchesKey         = bsonutil.MustHaveTag(Patch{}, "Patches")
	ActivatedKey       = bsonutil.MustHaveTag(Patch{}, "Activated")
	PatchedConfigKey   = bsonutil.MustHaveTag(Patch{}, "PatchedConfig")
	githubPatchDataKey = bsonutil.MustHaveTag(Patch{}, "GithubPatchData")

	// BSON fields for the module patch struct
	ModulePatchNameKey    = bsonutil.MustHaveTag(ModulePatch{}, "ModuleName")
	ModulePatchGithashKey = bsonutil.MustHaveTag(ModulePatch{}, "Githash")
	ModulePatchSetKey     = bsonutil.MustHaveTag(ModulePatch{}, "PatchSet")

	// BSON fields for the patch set struct
	PatchSetPatchKey   = bsonutil.MustHaveTag(PatchSet{}, "Patch")
	PatchSetSummaryKey = bsonutil.MustHaveTag(PatchSet{}, "Summary")

	// BSON fields for the git patch summary struct
	GitSummaryNameKey      = bsonutil.MustHaveTag(Summary{}, "Name")
	GitSummaryAdditionsKey = bsonutil.MustHaveTag(Summary{}, "Additions")
	GitSummaryDeletionsKey = bsonutil.MustHaveTag(Summary{}, "Deletions")

	// BSON fields for GithubPatch
	githubPatchPRNumberKey  = bsonutil.MustHaveTag(GithubPatch{}, "PRNumber")
	githubPatchBaseOwnerKey = bsonutil.MustHaveTag(GithubPatch{}, "BaseOwner")
	githubPatchBaseRepoKey  = bsonutil.MustHaveTag(GithubPatch{}, "BaseRepo")
	githubPatchHeadOwnerKey = bsonutil.MustHaveTag(GithubPatch{}, "HeadOwner")
	githubPatchHeadRepoKey  = bsonutil.MustHaveTag(GithubPatch{}, "HeadRepo")
	githubPatchHeadHashKey  = bsonutil.MustHaveTag(GithubPatch{}, "HeadHash")
	githubPatchAuthorKey    = bsonutil.MustHaveTag(GithubPatch{}, "Author")
	githubPatchDiffURLKey   = bsonutil.MustHaveTag(GithubPatch{}, "DiffURL")
)

// Query Validation

// IsValidId returns whether the supplied Id is a valid patch doc id (BSON ObjectId).
func IsValidId(id string) bool {
	return bson.IsObjectIdHex(id)
}

// NewId constructs a valid patch Id from the given hex string.
func NewId(id string) bson.ObjectId {
	return bson.ObjectIdHex(id)
}

// Queries

// ById produces a query to return the patch with the given _id.
func ById(id bson.ObjectId) db.Q {
	return db.Query(bson.M{IdKey: id})
}

// ByProject produces a query that returns projects with the given identifier.
func ByProject(project string) db.Q {
	return db.Query(bson.M{ProjectKey: project})
}

// ByUser produces a query that returns patches by the given user.
func ByUser(user string) db.Q {
	return db.Query(bson.M{AuthorKey: user})
}

// ByUserPaginated produces a query that returns patches by the given user
// before/after the input time, sorted by creation time and limited
func ByUserPaginated(user string, ts time.Time, limit int, sortAsc bool) db.Q {
	filter := bson.M{
		AuthorKey: user,
	}

	sortSpec := CreateTimeKey

	if !sortAsc {
		sortSpec = "-" + sortSpec
		filter[CreateTimeKey] = bson.M{"$lte": ts}
	} else {
		filter[CreateTimeKey] = bson.M{"$gt": ts}
	}
	return db.Query(filter).Sort([]string{sortSpec}).Limit(limit)
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
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return patch, err
}

// Find runs a patch query, returning all patches that satisfy the query.
func Find(query db.Q) ([]Patch, error) {
	patches := []Patch{}
	err := db.FindAllQ(Collection, query, &patches)
	if err == mgo.ErrNotFound {
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
func UpdateAll(query interface{}, update interface{}) (info *mgo.ChangeInfo, err error) {
	return db.UpdateAll(Collection, query, update)
}

// UpdateOne runs an update on a single patch document.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(Collection, query, update)
}

// PatchesByProject builds a query for patches that match the given
// project's id.
//
// If the sort value is less than 0, the query will return all
// matching patches that occur before the specified time, and otherwise
// will return all matching patches that occur after the specified time.
func PatchesByProject(projectId string, ts time.Time, limit int, sortAsc bool) db.Q {
	filter := bson.M{
		ProjectKey: projectId,
	}

	sortSpec := CreateTimeKey

	if !sortAsc {
		sortSpec = "-" + sortSpec
		filter[CreateTimeKey] = bson.M{"$lte": ts}
	} else {
		filter[CreateTimeKey] = bson.M{"$gt": ts}
	}
	return db.Query(filter).Sort([]string{sortSpec}).Limit(limit)
}

func ByGithubPRAndCreatedBefore(t time.Time, owner, repo string, prNumber int) db.Q {
	return db.Query(bson.M{
		CreateTimeKey: bson.M{
			"$lt": t,
		},
		bsonutil.GetDottedKeyName(githubPatchDataKey, githubPatchBaseOwnerKey): owner,
		bsonutil.GetDottedKeyName(githubPatchDataKey, githubPatchBaseRepoKey):  repo,
		bsonutil.GetDottedKeyName(githubPatchDataKey, githubPatchPRNumberKey):  prNumber,
	})
}
