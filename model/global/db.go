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

// GetNewBuildVariantBuildNumber atomically gets a new number for a build,
// given its variant name.
func GetNewBuildVariantBuildNumber(buildVariant string) (uint64, error) {
	session, db, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return 0, err
	}
	defer session.Close()

	// get the record for this build variant
	global := Global{}
	err = db.C(Collection).Find(bson.M{BuildVariantKey: buildVariant}).One(&global)

	// if this is the first build for this
	// this build variant, insert it
	if err != nil && adb.ResultsNotFound(err) {
		change := adb.Change{
			Update:    bson.M{"$inc": bson.M{LastBuildNumberKey: 1}},
			Upsert:    true,
			ReturnNew: true,
		}

		_, err = db.C(Collection).Find(bson.M{BuildVariantKey: buildVariant}).Apply(change, &global)
		if err != nil {
			return 0, err
		}
		return global.LastBuildNumber, nil
	}

	// At this point, we know we've
	// seen this build variant before
	// find and modify last build variant number
	change := adb.Change{
		Update:    bson.M{"$inc": bson.M{LastBuildNumberKey: 1}},
		ReturnNew: true,
	}

	_, err = db.C(Collection).Find(bson.M{BuildVariantKey: buildVariant}).Apply(change, &global)

	if err != nil {
		return 0, err
	}

	return global.LastBuildNumber, nil
}
