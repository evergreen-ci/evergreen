package definition

import (
	"context"
	"time"

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
func Find(ctx context.Context, q db.Q) ([]PodDefinition, error) {
	defs := []PodDefinition{}
	return defs, errors.WithStack(db.FindAllQ(ctx, Collection, q, &defs))
}

// FindOne finds one pod definition by the given query.
func FindOne(ctx context.Context, q db.Q) (*PodDefinition, error) {
	var def PodDefinition
	err := db.FindOneQ(ctx, Collection, q, &def)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &def, nil
}

// UpsertOne updates an existing pod definition if it exists based on the
// query; otherwise, it inserts a new pod definition.
func UpsertOne(ctx context.Context, query, update any) (*adb.ChangeInfo, error) {
	return db.Upsert(ctx, Collection, query, update)
}

// UpdateOne updates an existing pod definition.
func UpdateOne(ctx context.Context, query, update any) error {
	return db.Update(ctx, Collection, query, update)
}

// FindOneID returns a query to find a pod definition with the given ID.
func FindOneID(ctx context.Context, id string) (*PodDefinition, error) {
	return FindOne(ctx, db.Query(ByID(id)))
}

// ByID returns a query to find pod definitions with the given ID.
func ByID(id string) bson.M {
	return bson.M{IDKey: id}
}

// ByExternalID returns a query to find pod definitions with the given external
// ID.
func ByExternalID(id string) bson.M {
	return bson.M{ExternalIDKey: id}
}

// FindOneByExternalID find a pod definition with the given external ID.
func FindOneByExternalID(ctx context.Context, id string) (*PodDefinition, error) {
	return FindOne(ctx, db.Query(ByExternalID(id)))
}

// FindOneByFamily finds a pod definition with the given family name.
func FindOneByFamily(ctx context.Context, family string) (*PodDefinition, error) {
	return FindOne(ctx, db.Query(bson.M{
		FamilyKey: family,
	}))
}

// FindByLastAccessedBefore finds all pod definitions that were last accessed
// before the TTL. If a positive limit is given, it will return at most that
// number of results; otherwise, the results are unlimited.
func FindByLastAccessedBefore(ctx context.Context, ttl time.Duration, limit int) ([]PodDefinition, error) {
	return Find(ctx, db.Query(bson.M{
		"$or": []bson.M{
			{
				LastAccessedKey: bson.M{"$lt": time.Now().Add(-ttl)},
			},
			{
				LastAccessedKey: nil,
			},
		},
	}).Limit(limit))
}
