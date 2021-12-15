package global

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// Collection is the name of the MongoDB collection that stores global
	// internal tracking information.
	Collection = "globals"
)

var (
	BuildVariantKey    = bsonutil.MustHaveTag(Global{}, "BuildVariant")
	LastBuildNumberKey = bsonutil.MustHaveTag(Global{}, "LastBuildNumber")
)

// FindOne gets one global build variant document for the given query.
func FindOne(q bson.M) (*Global, error) {
	var global Global
	err := db.FindOneQ(Collection, db.Query(q), &global)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return &global, err
}

// GetNewBuildVariantBuildNumber atomically gets a new number for a build,
// given its variant name.
func GetNewBuildVariantBuildNumber(buildVariant string) (uint64, error) {
	change := adb.Change{
		Update:    bson.M{"$inc": bson.M{LastBuildNumberKey: 1}},
		Upsert:    true,
		ReturnNew: true,
	}

	var global Global
	if _, err := db.FindAndModify(Collection, bson.M{BuildVariantKey: buildVariant}, nil, change, &global); err != nil {
		return 0, err
	}

	return global.LastBuildNumber, nil
}
