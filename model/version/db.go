package version

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/db/bsonutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
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
	ProjectKey             = bsonutil.MustHaveTag(Version{}, "Project")
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
	OwnerNameKey           = bsonutil.MustHaveTag(Version{}, "Owner")
	RepoKey                = bsonutil.MustHaveTag(Version{}, "Repo")
	ProjectNameKey         = bsonutil.MustHaveTag(Version{}, "Branch")
	RepoKindKey            = bsonutil.MustHaveTag(Version{}, "RepoKind")
	ErrorsKey              = bsonutil.MustHaveTag(Version{}, "Errors")
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

// ByLastKnownGoodConfig filters on versions with valid (i.e., have no errors) config for the given
// project. Does not apply a limit, so should generally be used with a FindOne.
func ByLastKnownGoodConfig(projectId string) db.Q {
	return db.Query(
		bson.M{
			IdentifierKey: projectId,
			RequesterKey:  mci.RepotrackerVersionRequester,
			ErrorsKey: bson.M{
				"$exists": false,
			},
		}).Sort([]string{"-" + RevisionOrderNumberKey})
}

// ByProjectIdAndRevision finds non-patch versions for the given project and revision.
func ByProjectIdAndRevision(projectId, revision string) db.Q {
	return db.Query(
		bson.M{
			ProjectKey:   projectId,
			RevisionKey:  revision,
			RequesterKey: mci.RepotrackerVersionRequester,
		})
}

// ByLastVariantActivation finds the most recent non-patch versions in a project that have
// a particular variant activated.
func ByLastVariantActivation(projectId, variant string) db.Q {
	return db.Query(
		bson.M{
			ProjectKey:   projectId,
			RequesterKey: mci.RepotrackerVersionRequester,
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
			ProjectKey:   projectId,
			RequesterKey: mci.RepotrackerVersionRequester,
		})
}

// ByProjectId finds all versions within a project, ordered by most recently created to oldest.
// The requester controls if it should search patch or non-patch versions.
func ByMostRecentForRequester(projectId, requester string) db.Q {
	return db.Query(
		bson.M{
			RequesterKey: requester,
			ProjectKey:   projectId,
		},
	).Sort([]string{"-" + RevisionOrderNumberKey})
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
