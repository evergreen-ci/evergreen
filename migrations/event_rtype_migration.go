package migrations

import (
	"time"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	migrationEventRtypeRestructureAllLogs  = "event-rtype-to-root-alllogs"
	migrationEventRtypeRestructureTaskLogs = "event-rtype-to-root-tasklogs"
)

const migrationTime = "2015-10-21T16:29:00-07:00"

func makeEventRTypeMigration(collection string) migrationGeneratorFactory { //nolint: deadcode
	loc, _ := time.LoadLocation("UTC")
	bttf, err := time.ParseInLocation(time.RFC3339, migrationTime, loc)
	if err != nil {
		return func(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
			return nil, errors.Wrap(err, "time is invalid")
		}
	}

	return func(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
		const (
			resourceTypeKey = "r_type"
			dataKey         = "data"
			processedAtKey  = "processed_at"
		)
		var embeddedResourceTypeKey = bsonutil.GetDottedKeyName(dataKey, resourceTypeKey)

		notExists := bson.M{
			"$exists": false,
		}
		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         args.db,
				Collection: collection,
			},
			Limit: args.limit,
			Query: bson.M{
				processedAtKey:  notExists,
				resourceTypeKey: notExists,
			},
			JobID: args.id,
		}

		return anser.NewSimpleMigrationGenerator(env, opts, bson.M{
			"$set": bson.M{
				processedAtKey: bttf,
			},
			"$rename": bson.M{
				embeddedResourceTypeKey: resourceTypeKey,
			},
		}), nil
	}
}
