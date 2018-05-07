package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	perfCopyVariantMigrationName = "perf-copy-variant"
	jsonCollection               = "json"
	tagKey                       = "tag"
	projectIDKey                 = "project_id"
	variantKey                   = "variant"
	idKey                        = "_id"
)

type PerfCopyVariantArgs struct {
	Tag         string
	ProjectID   string
	FromVariant string
	ToVariant   string
}

func perfCopyVariantFactoryFactory(copyArgs PerfCopyVariantArgs) migrationGeneratorFactory {
	return func(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
		if err := env.RegisterManualMigrationOperation(perfCopyVariantMigrationName, makePerfCopyVariantMigration(args.db, copyArgs)); err != nil {
			return nil, err
		}

		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         args.db,
				Collection: jsonCollection,
			},
			Limit: args.limit,
			Query: bson.M{
				tagKey:       copyArgs.Tag,
				projectIDKey: copyArgs.ProjectID,
				variantKey:   copyArgs.FromVariant,
			},
			JobID: args.id,
		}

		return anser.NewManualMigrationGenerator(env, opts, perfCopyVariantMigrationName), nil
	}
}

func makePerfCopyVariantMigration(database string, copyArgs PerfCopyVariantArgs) db.MigrationOperation {
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()
		var id bson.ObjectId
		for _, raw := range rawD {
			if raw.Name == idKey {
				if err := raw.Value.Unmarshal(&id); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}
			}
		}
		doc := db.Document{}
		err := session.DB(database).C(jsonCollection).FindId(id).One(&doc)
		if err != nil {
			return errors.Wrapf(err, "problem finding document %s", id)
		}
		doc[variantKey] = copyArgs.ToVariant
		delete(doc, idKey)
		err = session.DB(database).C(jsonCollection).Insert(doc)
		if err != nil {
			return errors.Wrapf(err, "problem inserting copy of document %s", id)
		}
		return nil
	}
}
