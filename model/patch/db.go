package patch

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection   = "patches"
	GridFSPrefix = "patchfiles"
)

// BSON fields for the patches
var (
	IdKey            = bsonutil.MustHaveTag(Patch{}, "Id")
	DescriptionKey   = bsonutil.MustHaveTag(Patch{}, "Description")
	ProjectKey       = bsonutil.MustHaveTag(Patch{}, "Project")
	GithashKey       = bsonutil.MustHaveTag(Patch{}, "Githash")
	AuthorKey        = bsonutil.MustHaveTag(Patch{}, "Author")
	NumberKey        = bsonutil.MustHaveTag(Patch{}, "PatchNumber")
	VersionKey       = bsonutil.MustHaveTag(Patch{}, "Version")
	StatusKey        = bsonutil.MustHaveTag(Patch{}, "Status")
	CreateTimeKey    = bsonutil.MustHaveTag(Patch{}, "CreateTime")
	StartTimeKey     = bsonutil.MustHaveTag(Patch{}, "StartTime")
	FinishTimeKey    = bsonutil.MustHaveTag(Patch{}, "FinishTime")
	BuildVariantsKey = bsonutil.MustHaveTag(Patch{}, "BuildVariants")
	TasksKey         = bsonutil.MustHaveTag(Patch{}, "Tasks")
	PatchesKey       = bsonutil.MustHaveTag(Patch{}, "Patches")
	ActivatedKey     = bsonutil.MustHaveTag(Patch{}, "Activated")
	PatchedConfigKey = bsonutil.MustHaveTag(Patch{}, "PatchedConfig")

	// BSON fields for the module patch struct
	ModulePatchNameKey    = bsonutil.MustHaveTag(ModulePatch{}, "ModuleName")
	ModulePatchGithashKey = bsonutil.MustHaveTag(ModulePatch{}, "Githash")
	ModulePatchSetKey     = bsonutil.MustHaveTag(ModulePatch{}, "PatchSet")

	// BSON fields for the patch set struct
	PatchSetPatchKey   = bsonutil.MustHaveTag(PatchSet{}, "Patch")
	PatchSetSummaryKey = bsonutil.MustHaveTag(PatchSet{}, "Summary")

	// BSON fields for the git patch summary struct
	GitSummaryNameKey      = bsonutil.MustHaveTag(thirdparty.Summary{}, "Name")
	GitSummaryAdditionsKey = bsonutil.MustHaveTag(thirdparty.Summary{}, "Additions")
	GitSummaryDeletionsKey = bsonutil.MustHaveTag(thirdparty.Summary{}, "Deletions")
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
	return db.Query(bson.D{{IdKey, id}})
}

// ByProject produces a query that returns projects with the given identifier.
func ByProject(project string) db.Q {
	return db.Query(bson.D{{ProjectKey, project}})
}

// ByUser produces a query that returns patches by the given user.
func ByUser(user string) db.Q {
	return db.Query(bson.D{{AuthorKey, user}})
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
	return db.Query(bson.D{{VersionKey, version}})
}

// ByVersion produces a query that returns the patch for a given version.
func ByVersions(versions []string) db.Q {
	return db.Query(bson.M{VersionKey: bson.M{"$in": versions}})
}

// ExcludePatchDiff is a projection that excludes diff data, helping load times.
var ExcludePatchDiff = bson.D{
	{PatchesKey + "." + ModulePatchSetKey + "." + PatchSetPatchKey, 0},
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
