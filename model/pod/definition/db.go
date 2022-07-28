package definition

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
)

const Collection = "pod_definitions"

var (
	IDKey           = bsonutil.MustHaveTag(PodDefinition{}, "ID")
	ExternalIDKey   = bsonutil.MustHaveTag(PodDefinition{}, "ExternalID")
	DigestKey       = bsonutil.MustHaveTag(PodDefinition{}, "Digest")
	LastAccessedKey = bsonutil.MustHaveTag(PodDefinition{}, "LastAccessed")
)

// Find finds all pod definitions matching the given query.
func Find(q db.Q) ([]PodDefinition, error) {
	defs := []PodDefinition{}
	return defs, errors.WithStack(db.FindAllQ(Collection, q, &defs))
}

// FindOne finds one pod definition by the given query.
func FindOne(q db.Q) (*PodDefinition, error) {
	var def PodDefinition
	err := db.FindOneQ(Collection, q, &def)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &def, nil
}

// UpsertOne updates an existing pod definition if it exists based on the
// query; otherwise, it inserts a new pod definition.
func UpsertOne(query, update interface{}) (*adb.ChangeInfo, error) {
	return db.Upsert(Collection, query, update)
}

// FindOneID returns a query to find a pod definition with the given ID.
func FindOneID(id string) (*PodDefinition, error) {
	return FindOne(db.Query(bson.M{
		IDKey: id,
	}))
}

// ByExternalID returns a query to find pod definitions with the given external
// ID.
func ByExternalID(id string) bson.M {
	return bson.M{ExternalIDKey: id}
}

// FindOneByExternalID find a pod definition with the given external ID.
func FindOneByExternalID(id string) (*PodDefinition, error) {
	return FindOne(db.Query(ByExternalID(id)))
}

// FindOneByDigest returns a query to find a pod definition with a matching
// hash digest.
func FindOneByDigest(digest string) (*PodDefinition, error) {
	return FindOne(db.Query(bson.M{
		DigestKey: digest,
	}))
}
