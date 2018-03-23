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

const migrationTime = "21 Oct 15 16:29 PDT"

func makeEventRTypeMigration(collection string) migrationGeneratorFactory { //nolint: deadcode
	bttf, err := time.Parse(time.RFC822, migrationTime)
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

		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         args.db,
				Collection: collection,
			},
			Limit: args.limit,
			Query: bson.M{
				processedAtKey: bson.M{
					"$exists": false,
					"$ne":     bttf,
				},
				resourceTypeKey: bson.M{
					"$exists": false,
				},
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
