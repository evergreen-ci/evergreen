package version

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection = "versions"
)

var (
	// bson fields for the version struct
	IdKey                  = bsonutil.MustHaveTag(Version{}, "Id")
	CreateTimeKey          = bsonutil.MustHaveTag(Version{}, "CreateTime")
	StartTimeKey           = bsonutil.MustHaveTag(Version{}, "StartTime")
	FinishTimeKey          = bsonutil.MustHaveTag(Version{}, "FinishTime")
	RevisionKey            = bsonutil.MustHaveTag(Version{}, "Revision")
	AuthorKey              = bsonutil.MustHaveTag(Version{}, "Author")
	AuthorEmailKey         = bsonutil.MustHaveTag(Version{}, "AuthorEmail")
	MessageKey             = bsonutil.MustHaveTag(Version{}, "Message")
	StatusKey              = bsonutil.MustHaveTag(Version{}, "Status")
	BuildIdsKey            = bsonutil.MustHaveTag(Version{}, "BuildIds")
	BuildVariantsKey       = bsonutil.MustHaveTag(Version{}, "BuildVariants")
	RevisionOrderNumberKey = bsonutil.MustHaveTag(Version{}, "RevisionOrderNumber")
	RequesterKey           = bsonutil.MustHaveTag(Version{}, "Requester")
	ConfigKey              = bsonutil.MustHaveTag(Version{}, "Config")
	IgnoredKey             = bsonutil.MustHaveTag(Version{}, "Ignored")
	OwnerNameKey           = bsonutil.MustHaveTag(Version{}, "Owner")
	RepoKey                = bsonutil.MustHaveTag(Version{}, "Repo")
	ProjectNameKey         = bsonutil.MustHaveTag(Version{}, "Branch")
	RepoKindKey            = bsonutil.MustHaveTag(Version{}, "RepoKind")
	ErrorsKey              = bsonutil.MustHaveTag(Version{}, "Errors")
	WarningsKey            = bsonutil.MustHaveTag(Version{}, "Warnings")
	IdentifierKey          = bsonutil.MustHaveTag(Version{}, "Identifier")
	RemoteKey              = bsonutil.MustHaveTag(Version{}, "Remote")
	RemoteURLKey           = bsonutil.MustHaveTag(Version{}, "RemotePath")
)

// ById returns a db.Q object which will filter on {_id : <the id param>}
func ById(id string) db.Q {
	return db.Query(bson.M{IdKey: id})
}

// ByIds returns a db.Q object which will find any versions whose _id appears in the given list.
func ByIds(ids []string) db.Q {
	return db.Query(bson.M{IdKey: bson.M{"$in": ids}})
}

// All is a query for all versions.
var All = db.Query(bson.D{})

// ByLastKnownGoodConfig filters on versions with valid (i.e., have no errors) config for the given
// project. Does not apply a limit, so should generally be used with a FindOne.
func ByLastKnownGoodConfig(projectId string) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			RequesterKey:  evergreen.RepotrackerVersionRequester,
			ErrorsKey: bson.M{
				"$exists": false,
			},
		}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByProjectIdAndRevision finds non-patch versions for the given project and revision.
func ByProjectIdAndRevision(projectId, revision string) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			RevisionKey:   revision,
			RequesterKey:  evergreen.RepotrackerVersionRequester,
		})
}

func ByProjectIdAndRevisionPrefix(projectId, revisionPrefix string) db.Q {
	lengthHash := (40 - len(revisionPrefix))
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			RevisionKey:   bson.M{"$regex": fmt.Sprintf("^%s[0-9a-f]{%d}$", revisionPrefix, lengthHash)},
			RequesterKey:  evergreen.RepotrackerVersionRequester,
		})
}

// ByProjectIdAndOrder finds non-patch versions for the given project with revision
// order numbers less than or equal to revisionOrderNumber.
func ByProjectIdAndOrder(projectId string, revisionOrderNumber int) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey:          projectId,
			RevisionOrderNumberKey: bson.M{"$lte": revisionOrderNumber},
			RequesterKey:           evergreen.RepotrackerVersionRequester,
		})
}

// ByLastVariantActivation finds the most recent non-patch, non-ignored
// versions in a project that have a particular variant activated.
func ByLastVariantActivation(projectId, variant string) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			// TODO make this `Ignored: false` after EVG-764  has time to burn in
			IgnoredKey:   bson.M{"$ne": true},
			RequesterKey: evergreen.RepotrackerVersionRequester,
			BuildVariantsKey: bson.M{
				"$elemMatch": bson.M{
					BuildStatusActivatedKey: true,
					BuildStatusVariantKey:   variant,
				},
			},
		},
	).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByProjectId finds all non-patch versions within a project.
func ByProjectId(projectId string) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			RequesterKey:  evergreen.RepotrackerVersionRequester,
		})
}

// ByProjectId finds all versions within a project, ordered by most recently created to oldest.
// The requester controls if it should search patch or non-patch versions.
func ByMostRecentForRequester(projectId, requester string) db.Q {
	return db.Query(
		bson.M{
			RequesterKey:  requester,
			IdentifierKey: projectId,
		},
	).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByMostRecentNonignored finds all non-ignored versions within a project,
// ordered by most recently created to oldest.
func ByMostRecentNonignored(projectId string) db.Q {
	return db.Query(
		bson.M{
			RequesterKey:  evergreen.RepotrackerVersionRequester,
			IdentifierKey: projectId,
			IgnoredKey:    bson.M{"$ne": true},
		},
	).Sort([]string{"-" + RevisionOrderNumberKey})
}

// BaseVersionFromPatch finds the base version for a patch version.
func BaseVersionFromPatch(projectId, revision string) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			RevisionKey:   revision,
			RequesterKey:  evergreen.RepotrackerVersionRequester,
		})
}

func FindOne(query db.Q) (*Version, error) {
	version := &Version{}
	err := db.FindOneQ(Collection, query, version)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return version, err
}

func Find(query db.Q) ([]Version, error) {
	versions := []Version{}
	err := db.FindAllQ(Collection, query, &versions)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return versions, err
}

// Count returns the number of hosts that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}

// UpdateOne updates one version.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

// RemoveOne removes one version
func RemoveOne(query interface{}) error {
	return db.Remove(Collection, query)
}
