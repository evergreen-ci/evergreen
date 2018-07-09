package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	migrationDistroSecurityGroups = "distro-security-groups"
	distroCollection              = "distro"
	settingsKey                   = "settings"
	securityGroupKey              = "security_group"
	securityGroupsKey             = "security_group_ids"
)

type providerSettings struct {
	SecurityGroup  string   `bson:"security_group"`
	SecurityGroups []string `bson:"security_group_ids"`
}

func distroSecurityGroupsGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {

	if err := env.RegisterManualMigrationOperation(migrationDistroSecurityGroups, makeDistroSecurityGroupMigration(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: distroCollection,
		},
		Limit: args.limit,
		Query: db.Document{
			bsonutil.GetDottedKeyName(settingsKey, securityGroupKey): db.Document{
				"$exists": true,
			},
		},
		JobID: args.id,
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationDistroSecurityGroups), nil
}

func makeDistroSecurityGroupMigration(database string) db.MigrationOperation {
	const (
		idKey = "_id"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		var id string
		var settings providerSettings
		for _, raw := range rawD {
			switch raw.Name {
			case idKey:
				if err := raw.Value.Unmarshal(&id); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}
			case settingsKey:
				if err := raw.Value.Unmarshal(&settings); err != nil {
					return errors.Wrap(err, "error unmarshaling settings")
				}
			}
		}

		update := db.Document{
			"$push": db.Document{
				bsonutil.GetDottedKeyName(settingsKey, securityGroupsKey): settings.SecurityGroup,
			},
			"$unset": db.Document{
				bsonutil.GetDottedKeyName(settingsKey, securityGroupKey): "",
			},
		}
		return session.DB(database).C(distroCollection).UpdateId(id, update)
	}
}
