package migrations

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	eventCollection       = "event_log"
	adminDataType         = "ADMIN"
	idKey                 = "_id"
	dataKey               = "data"
	eventTypeKey          = "e_type"
	eventTypeBanner       = "BANNER_CHANGED"
	eventTypeTheme        = "THEME_CHANGED"
	eventTypeServiceFlags = "SERVICE_FLAGS_CHANGED"
)

type EventDataOld struct {
	ResourceType string                 `bson:"r_type"`
	User         string                 `bson:"user"`
	OldVal       string                 `bson:"old_val"`
	NewVal       string                 `bson:"new_val"`
	OldFlags     evergreen.ServiceFlags `bson:"old_flags"`
	NewFlags     evergreen.ServiceFlags `bson:"new_flags"`
}

func adminEventRestructureGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "admin_event_restructure"

	if err := env.RegisterManualMigrationOperation(migrationName, makeAdminEventMigration(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: eventCollection,
		},
		Limit: limit,
		Query: bson.M{
			"r_id":        "",
			"data.r_type": adminDataType,
		},
		JobID: "migration-admin-event-restructure",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeAdminEventMigration(database string) db.MigrationOperation {
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		var docId bson.ObjectId
		oldData := EventDataOld{}
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

		newData := event.AdminEventData{
			ResourceType: oldData.ResourceType,
			User:         oldData.User,
		}
		switch changeType {
		case eventTypeTheme:
			before := &evergreen.Settings{BannerTheme: evergreen.BannerTheme(oldData.OldVal)}
			after := &evergreen.Settings{BannerTheme: evergreen.BannerTheme(oldData.NewVal)}
			newData.Section = before.SectionId()
			newData.Changes = event.ConfigDataChange{Before: before, After: after}
		case eventTypeBanner:
			before := &evergreen.Settings{Banner: oldData.OldVal}
			after := &evergreen.Settings{Banner: oldData.NewVal}
			newData.Section = before.SectionId()
			newData.Changes = event.ConfigDataChange{Before: before, After: after}
		case eventTypeServiceFlags:
			before := oldData.OldFlags
			after := oldData.NewFlags
			newData.Section = before.SectionId()
			newData.Changes = event.ConfigDataChange{Before: &before, After: &after}
		}

		return session.DB(database).C(eventCollection).UpdateId(docId,
			bson.M{
				"$set": bson.M{
					dataKey:      newData,
					eventTypeKey: event.EventTypeValueChanged,
				},
			})
	}
}
