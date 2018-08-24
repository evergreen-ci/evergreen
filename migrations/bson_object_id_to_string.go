package migrations

import (
	"fmt"

	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	bsonObjectIDmigrationNameTmpl             = "%s-collection-bson-objectid-to-string"
	migrationSubscriptionBSONObjectIDToString = "subscriptions-collection-bson-objectid-to-string"
)

func makeBSONObjectIDToStringGenerator(collection string) migrationGeneratorFactory {
	return func(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
		migrationName := fmt.Sprintf(bsonObjectIDmigrationNameTmpl, collection)
		if err := env.RegisterManualMigrationOperation(migrationName, makeBSONObjectIDToStringMigration(args.db, collection)); err != nil {
			return nil, err
		}

		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         args.db,
				Collection: collection,
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
}

func makeBSONObjectIDToStringMigration(database, collection string) db.MigrationOperation {
	const idKey = "_id"
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
		err := session.DB(database).C(collection).Insert(doc)
		if err != nil {
			return err
		}

		return session.DB(database).C(collection).RemoveId(v)
	}
}
