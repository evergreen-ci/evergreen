package migrations

import (
	"time"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/anser/model"
	"gopkg.in/mgo.v2/bson"
)

type eventRType struct {
	ID   bson.ObjectId  `bson:"_id"`
	Data eventDataRType `bson:"data"`
}

type eventDataRType struct {
	ResourceType string `bson:"r_type"`
}

func makeEventRTypeMigration(collection string) migrationGeneratorFactory {
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
				embeddedResourceTypeKey: bson.M{
					"$exists": true,
				},
			},
			JobID: "migration-event-rtype-to-root",
		}

		return anser.NewSimpleMigrationGenerator(env, opts, bson.M{
			"$set": bson.M{
				processedAtKey: time.Now(),
			},
			"$rename": bson.M{
				embeddedResourceTypeKey: resourceTypeKey,
			},
		}), nil
	}
}
