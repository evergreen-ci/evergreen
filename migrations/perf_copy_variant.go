package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	perfCopyVariantMigrationName = "perf-copy-variant"
	jsonCollection               = "json"
	idKey                        = "_id"
	perfTagKey                   = "tag"
	perfProjectIDKey             = "project_id"
	perfFromVariantKey           = "from_variant"
	perfToVariantKey             = "to_variant"
	perfVariantKey               = "variant"
)

func perfCopyVariantFactoryFactory(args map[string]string) migrationGeneratorFactory {
	return func(env anser.Environment, generatorArgs migrationGeneratorFactoryOptions) (anser.Generator, error) {
		if err := env.RegisterManualMigrationOperation(perfCopyVariantMigrationName, makePerfCopyVariantMigration(generatorArgs.db, args)); err != nil {
			return nil, err
		}

		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         generatorArgs.db,
				Collection: jsonCollection,
			},
			Limit: generatorArgs.limit,
			Query: bson.M{
				perfTagKey:       args[perfTagKey],
				perfProjectIDKey: args[perfProjectIDKey],
				perfVariantKey:   args[perfFromVariantKey],
			},
			JobID: generatorArgs.id,
		}

		return anser.NewManualMigrationGenerator(env, opts, perfCopyVariantMigrationName), nil
	}
}

func makePerfCopyVariantMigration(database string, args map[string]string) db.MigrationOperation {
	return func(session db.Session, rawD mgobson.RawD) error {
		defer session.Close()
		var id mgobson.ObjectId
		for _, raw := range rawD {
			if raw.Name == idKey {
				if err := raw.Value.Unmarshal(&id); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}
			}
		}
		if !id.Valid() {
			return errors.Errorf("id %s is not valid", id)
		}
		doc := db.Document{}
		err := session.DB(database).C(jsonCollection).FindId(id).One(&doc)
		if err != nil {
			return errors.Wrapf(err, "problem finding document %s", id)
		}
		doc[perfVariantKey] = args[perfToVariantKey]
		delete(doc, idKey)
		err = session.DB(database).C(jsonCollection).Insert(doc)
		if err != nil {
			return errors.Wrapf(err, "problem inserting copy of document %s", id)
		}
		return nil
	}
}
