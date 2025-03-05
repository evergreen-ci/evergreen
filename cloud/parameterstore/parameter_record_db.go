package parameterstore

import (
	"context"
	"time"

	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Collection holds metadata information about parameters.
const Collection = "parameter_records"

var (
	nameKey        = bsonutil.MustHaveTag(ParameterRecord{}, "Name")
	lastUpdatedKey = bsonutil.MustHaveTag(ParameterRecord{}, "LastUpdated")
)

// BumpParameterRecord bumps the parameter record to indicate that the parameter
// was changed. information. It will not modify the record if it already exists
// and its latest update is more recent than lastUpdated.
func BumpParameterRecord(ctx context.Context, db *mongo.Database, name string, lastUpdated time.Time) error {
	res, err := db.Collection(Collection).UpdateOne(ctx, bson.M{
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
	}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 && res.UpsertedCount == 0 {
		return errors.Errorf("parameter record '%s' not upserted or modified", name)
	}
	return nil
}

// FindOneName finds one parameter record by its parameter name.
func FindOneName(ctx context.Context, db *mongo.Database, name string) (*ParameterRecord, error) {
	var p ParameterRecord
	err := db.Collection(Collection).FindOne(ctx, bson.M{nameKey: name}).Decode(&p)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByNames finds all parameter records by their parameter names.
func FindByNames(ctx context.Context, db *mongo.Database, names ...string) ([]ParameterRecord, error) {
	if len(names) == 0 {
		return nil, nil
	}
	params := []ParameterRecord{}
	cur, err := db.Collection(Collection).Find(ctx, bson.M{nameKey: bson.M{"$in": names}})
	if err != nil {
		return nil, err
	}
	if err := cur.All(ctx, &params); err != nil {
		return nil, err
	}
	return params, nil
}
