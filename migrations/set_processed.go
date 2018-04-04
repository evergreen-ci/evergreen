package migrations

import (
	"time"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	migrationEventSetProcessedTime = "event-set-processed-time"
)

const unsubscribableTime = "2015-10-21T16:29:01-07:00"

func makeEventSetProcesedTimeMigration(collection string, left, right time.Time) migrationGeneratorFactory { //nolint: deadcode
	loc, _ := time.LoadLocation("UTC")
	bttf, err := time.ParseInLocation(time.RFC3339, unsubscribableTime, loc)
	if err != nil {
		return func(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
			return nil, errors.Wrap(err, "time is invalid")
		}
	}

	return func(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
		const (
			processedAtKey = "processed_at"
			timeKey        = "ts"
		)

		q := bson.M{
			processedAtKey: bson.M{
				"$exists": false,
			},
		}

		if !left.IsZero() {
			q[timeKey] = bson.M{"$gte": left}
		}
		if !right.IsZero() {
			if _, ok := q[timeKey]; !ok {
				q[timeKey] = bson.M{}
			}

			q[timeKey].(bson.M)["$lte"] = right
		}

		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         args.db,
				Collection: collection,
			},
			Limit: args.limit,
			Query: q, JobID: args.id,
		}

		return anser.NewSimpleMigrationGenerator(env, opts, bson.M{
			"$set": bson.M{
				processedAtKey: bttf,
			},
		}), nil
	}
}
