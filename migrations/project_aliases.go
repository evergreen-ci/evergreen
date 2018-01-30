package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	projectVarsCollection  = "project_vars"
	projectAliasCollection = "project_aliases"
)

type patchDefinition struct {
	Alias   string   `bson:"alias"`
	Variant string   `bson:"variant"`
	Task    string   `bson:"task"`
	Tags    []string `bson:"tags"`
}

func projectAliasesToCollectionGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "project_aliases_to_collection"

	if err := env.RegisterManualMigrationOperation(migrationName, makeProjectAliasMigration(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: projectVarsCollection,
		},
		Limit: limit,
		//Query: bson.M{
		//	"patch_definitions": bson.M{
		//		"$not": bson.M{
		//			"$size": 0,
		//		},
		//	},
		//},
		JobID: "migration-project-aliases-to-collection",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeProjectAliasMigration(database string) db.MigrationOperation {
	const (
		idKey               = "_id"
		patchDefinitionsKey = "patch_definitions"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		projectID := ""
		aliases := []patchDefinition{}
		for _, raw := range rawD {
			switch raw.Name {
			case idKey:
				if err := raw.Value.Unmarshal(&projectID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case patchDefinitionsKey:
				if err := raw.Value.Unmarshal(&aliases); err != nil {
					return errors.Wrap(err, "error unmarshaling patch aliases")
				}
			}
		}

		catcher := grip.NewSimpleCatcher()
		for _, alias := range aliases {
			catcher.Add(session.DB(database).C(projectAliasCollection).Insert(bson.M{
				"_id":        bson.NewObjectId(),
				"project_id": projectID,
				"alias":      alias.Alias,
				"variant":    alias.Variant,
				"task":       alias.Task,
				"tags":       alias.Tags,
			}))
		}

		if catcher.HasErrors() {
			return catcher.Resolve()
		}

		return session.DB(database).C(projectVarsCollection).UpdateId(projectID,
			bson.M{
				"$unset": bson.M{
					patchDefinitionsKey: 1,
				},
			})
	}
}
