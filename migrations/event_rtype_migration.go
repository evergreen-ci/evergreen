package migrations

import (
	"time"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/anser/model"
	"gopkg.in/mgo.v2/bson"
)

func makeEventRTypeMigration(collection string) migrationGeneratorFactory { //nolint: deadcode
	nowTime := time.Now()

	return func(env anser.Environment, db string, limit int) (anser.Generator, error) {
		const (
			resourceTypeKey = "r_type"
			dataKey         = "data"
			processedAtKey  = "processed_at"
		)
		var embeddedResourceTypeKey = bsonutil.GetDottedKeyName(dataKey, resourceTypeKey)
		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         db,
				Collection: collection,
			},
			Limit: limit,
			Query: bson.M{
				resourceTypeKey: bson.M{
					"$exists": false,
				},
			},
			JobID: "migration-event-rtype-to-root",
		}

		return anser.NewSimpleMigrationGenerator(env, opts, bson.M{
			"$set": bson.M{
				processedAtKey: nowTime,
			},
			"$rename": bson.M{
				embeddedResourceTypeKey: resourceTypeKey,
			},
		}), nil
	}
}
