package parameterstore

import (
	"context"
	"time"

	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// collection holds metadata information about parameters.
const collection = "parameter_records"

var (
	nameKey        = bsonutil.MustHaveTag(parameterRecord{}, "Name")
	lastUpdatedKey = bsonutil.MustHaveTag(parameterRecord{}, "LastUpdated")
)

// bumpParameterRecord bumps the parameter record to indicate that the parameter
// was changed. information. This will not modify if the record exists and its
// most recent update is more recent than lastUpdated.
// kim: TODO: test
func bumpParameterRecord(ctx context.Context, db *mongo.Database, name string, lastUpdated time.Time) error {
	res, err := db.Collection(collection).UpdateOne(ctx, bson.M{
		nameKey: name,
		// Ensure that the latest parameter update wins (i.e. the update can
		// only bump it forward in time and it can never go backwards).
		lastUpdatedKey: bson.M{"$lt": lastUpdated},
	}, bson.M{
		"$set": bson.M{
			lastUpdatedKey: lastUpdated,
		},
		"$setOnInsert": bson.M{
			nameKey: name,
		},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 {
		return errors.Errorf("parameter record '%s' not updated", name)
	}
	return nil
}

// findOneID finds one parameter record by its parameter name.
func findOneID(ctx context.Context, db *mongo.Database, name string) (*parameterRecord, error) {
	var p parameterRecord
	err := db.Collection(collection).FindOne(ctx, bson.M{nameKey: name}).Decode(&p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// findByIDs finds all parameter records by their parameter names.
// kim: TODO: test
func findByIDs(ctx context.Context, db *mongo.Database, names ...string) ([]parameterRecord, error) {
	if len(names) == 0 {
		return nil, nil
	}
	params := []parameterRecord{}
	cur, err := db.Collection(collection).Find(ctx, bson.M{nameKey: bson.M{"$in": names}})
	if err != nil {
		return nil, err
	}
	if err := cur.All(ctx, &params); err != nil {
		return nil, err
	}
	return params, nil
}
