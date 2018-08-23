package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func subscriptionIDToStringGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
	const (
		migrationName          = "subscription-id-to-string"
		subscriptionCollection = "subscriptions"
	)

	if err := env.RegisterManualMigrationOperation(migrationName, makeSubscriptionIDToStringMigration(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: subscriptionCollection,
		},
		Limit: args.limit,
		Query: bson.M{
			"_id": bson.M{
				"$type": "objectId",
			},
		},
		JobID: args.id,
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeSubscriptionIDToStringMigration(database string) db.MigrationOperation {
	const (
		idKey                  = "_id"
		subscriptionCollection = "subscriptions"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		doc := db.Document{}
		for i := range rawD {
			var temp interface{}
			if err := rawD[i].Value.Unmarshal(&temp); err != nil {
				return errors.Wrapf(err, "Failed to unmarshal %s", rawD[i].Name)
			}
			doc[rawD[i].Name] = temp
		}
		v, ok := doc[idKey].(bson.ObjectId)
		if !ok || !v.Valid() {
			return errors.Errorf("%v is not a BSON object ID", doc[idKey])
		}

		doc[idKey] = v.Hex()
		err := session.DB(database).C(subscriptionCollection).Insert(doc)
		if err != nil {
			return err
		}

		return session.DB(database).C(subscriptionCollection).RemoveId(v)
	}
}
