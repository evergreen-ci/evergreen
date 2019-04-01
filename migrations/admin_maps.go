package migrations

import (
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	adminCollection              = "admin"
	globalDocId                  = "global"
	migrationAdminMapRestructure = "admin_map_restructure"
)

func adminMapRestructureGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
	if err := env.RegisterManualMigrationOperation(migrationAdminMapRestructure, makeAdminMapMigration(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: adminCollection,
		},
		Limit: args.limit,
		Query: db.Document{
			"_id": globalDocId,
		},
		JobID: args.id,
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationAdminMapRestructure), nil
}

func makeAdminMapMigration(database string) db.MigrationOperation {
	const (
		credentialsKey = "credentials"
		expansionsKey  = "expansions"
		keysKey        = "keys"
		pluginsKey     = "plugins"

		newCredentialsKey = "credentials_new"
		newExpansionsKey  = "expansions_new"
		newKeysKey        = "keys_new"
		newPluginsKey     = "plugins_new"
	)

	return func(session db.Session, rawD mgobson.RawD) error {
		defer session.Close()

		var credentials map[string]string
		var expansions map[string]string
		var keys map[string]string
		var plugins map[string]map[string]interface{}
		for _, raw := range rawD {
			switch raw.Name {
			case credentialsKey:
				if err := raw.Value.Unmarshal(&credentials); err != nil {
					return errors.Wrap(err, "error unmarshaling credentials")
				}
			case expansionsKey:
				if err := raw.Value.Unmarshal(&expansions); err != nil {
					return errors.Wrap(err, "error unmarshaling expansions")
				}
			case keysKey:
				if err := raw.Value.Unmarshal(&keys); err != nil {
					return errors.Wrap(err, "error unmarshaling keys")
				}
			case pluginsKey:
				if err := raw.Value.Unmarshal(&plugins); err != nil {
					return errors.Wrap(err, "error unmarshaling plugins")
				}
			}
		}

		newCredentials := util.KeyValuePairSlice{}
		for k, v := range credentials {
			newCredentials = append(newCredentials, util.KeyValuePair{Key: k, Value: v})
		}
		newExpansions := util.KeyValuePairSlice{}
		for k, v := range expansions {
			newExpansions = append(newExpansions, util.KeyValuePair{Key: k, Value: v})
		}
		newKeys := util.KeyValuePairSlice{}
		for k, v := range keys {
			newKeys = append(newKeys, util.KeyValuePair{Key: k, Value: v})
		}
		newPlugins := util.KeyValuePairSlice{}
		for k, v := range plugins {
			newMap := util.KeyValuePairSlice{}
			for k2, v2 := range v {
				newMap = append(newMap, util.KeyValuePair{Key: k2, Value: v2})
			}
			newPlugins = append(newPlugins, util.KeyValuePair{Key: k, Value: newMap})
		}

		return session.DB(database).C(adminCollection).UpdateId(globalDocId,
			db.Document{
				"$set": db.Document{
					newCredentialsKey: newCredentials,
					newExpansionsKey:  newExpansions,
					newKeysKey:        newKeys,
					newPluginsKey:     newPlugins,
				},
				"$unset": db.Document{
					credentialsKey: 1,
					expansionsKey:  1,
					keysKey:        1,
					pluginsKey:     1,
				},
			})
	}
}
