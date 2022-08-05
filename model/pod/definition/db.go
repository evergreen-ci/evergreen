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
	FamilyKey       = bsonutil.MustHaveTag(PodDefinition{}, "Family")
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

// UpdateOne updates an existing pod definition.
func UpdateOne(query, update interface{}) error {
	return db.Update(Collection, query, update)
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

// FindOneByFamily finds a pod definition with the given family name.
func FindOneByFamily(family string) (*PodDefinition, error) {
	return FindOne(db.Query(bson.M{
		FamilyKey: family,
	}))
}
