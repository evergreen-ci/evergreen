package migrations

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	eventCollection       = "event_log"
	adminDataType         = "ADMIN"
	eventTypeKey          = "e_type"
	eventTypeBanner       = "BANNER_CHANGED"
	eventTypeTheme        = "THEME_CHANGED"
	eventTypeServiceFlags = "SERVICE_FLAGS_CHANGED"
	eventTypeValueChanged = "CONFIG_VALUE_CHANGED"
)

type eventDataOld struct {
	ResourceType string      `bson:"r_type"`
	User         string      `bson:"user"`
	OldVal       string      `bson:"old_val"`
	NewVal       string      `bson:"new_val"`
	OldFlags     db.Document `bson:"old_flags"`
	NewFlags     db.Document `bson:"new_flags"`
}

type eventDataNew struct {
	ResourceType string           `bson:"r_type"`
	User         string           `bson:"user"`
	Section      string           `bson:"section"`
	Changes      configDataChange `bson:"changes"`
	GUID         string           `bson:"guid"`
}

type configDataChange struct {
	Before db.Document `bson:"before"`
	After  db.Document `bson:"after"`
}

func adminEventRestructureGenerator(env anser.Environment, dbName string, limit int) (anser.Generator, error) {
	const migrationName = "admin_event_restructure"

	if err := env.RegisterManualMigrationOperation(migrationName, makeAdminEventMigration(dbName)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         dbName,
			Collection: eventCollection,
		},
		Limit: limit,
		Query: db.Document{
			"r_id":        "",
			"data.r_type": adminDataType,
			"$or": []db.Document{
				{
					"data.old_val": db.Document{
						"$exists": true,
					},
				},
				{
					"data.new_val": db.Document{
						"$exists": true,
					},
				},
				{
					"data.old_flags": db.Document{
						"$exists": true,
					},
				},
				{
					"data.new_flags": db.Document{
						"$exists": true,
					},
				},
			},
		},
		JobID: "migration-admin-event-restructure",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeAdminEventMigration(database string) db.MigrationOperation {
	const (
		idKey   = "_id"
		dataKey = "data"
	)

	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		var docId bson.ObjectId
		oldData := eventDataOld{}
		changeType := ""
		for _, raw := range rawD {
			switch raw.Name {
			case idKey:
				if err := raw.Value.Unmarshal(&docId); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}
			case eventTypeKey:
				if err := raw.Value.Unmarshal(&changeType); err != nil {
					return errors.Wrap(err, "error unmarshaling event type")
				}
			case dataKey:
				if err := raw.Value.Unmarshal(&oldData); err != nil {
					return errors.Wrap(err, "error unmarshaling event data")
				}
			}
		}
		if changeType == "" {
			return errors.New("change type is empty")
		}

		newData := eventDataNew{
			ResourceType: oldData.ResourceType,
			User:         oldData.User,
			GUID:         util.RandomString(),
		}
		switch changeType {
		case eventTypeTheme:
			before := db.Document{"banner_theme": oldData.OldVal}
			after := db.Document{"banner_theme": oldData.NewVal}
			newData.Section = "global"
			newData.Changes = configDataChange{Before: before, After: after}
		case eventTypeBanner:
			before := db.Document{"banner": oldData.OldVal}
			after := db.Document{"banner": oldData.NewVal}
			newData.Section = "global"
			newData.Changes = configDataChange{Before: before, After: after}
		case eventTypeServiceFlags:
			beforeBytes, err := bson.Marshal(oldData.OldFlags)
			if err != nil {
				return err
			}
			afterBytes, err := bson.Marshal(oldData.NewFlags)
			if err != nil {
				return err
			}
			after := db.Document{}
			before := db.Document{}
			if err = bson.Unmarshal(beforeBytes, &before); err != nil {
				return err
			}
			if err = bson.Unmarshal(afterBytes, &after); err != nil {
				return err
			}
			newData.Section = "service_flags"
			newData.Changes = configDataChange{Before: before, After: after}
		default:
			return fmt.Errorf("unexpected change type %s found", changeType)
		}

		return session.DB(database).C(eventCollection).UpdateId(docId,
			db.Document{
				"$set": db.Document{
					dataKey:      newData,
					eventTypeKey: eventTypeValueChanged,
				},
			})
	}
}
