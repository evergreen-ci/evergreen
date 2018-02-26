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
	eventCollection       = "event_log"
	adminDataType         = "ADMIN"
	eventTypeKey          = "e_type"
	eventTypeBanner       = "BANNER_CHANGED"
	eventTypeTheme        = "THEME_CHANGED"
	eventTypeServiceFlags = "SERVICE_FLAGS_CHANGED"
	eventTypeValueChanged = "CONFIG_VALUE_CHANGED"
)

type eventDataOld struct {
	ResourceType string            `bson:"r_type"`
	User         string            `bson:"user"`
	OldVal       string            `bson:"old_val"`
	NewVal       string            `bson:"new_val"`
	OldFlags     eventServiceFlags `bson:"old_flags"`
	NewFlags     eventServiceFlags `bson:"new_flags"`
}

type eventServiceFlags struct {
	TaskDispatchDisabled         bool `bson:"task_dispatch_disabled"`
	HostinitDisabled             bool `bson:"hostinit_disabled"`
	MonitorDisabled              bool `bson:"monitor_disabled"`
	NotificationsDisabled        bool `bson:"notifications_disabled"`
	AlertsDisabled               bool `bson:"alerts_disabled"`
	TaskrunnerDisabled           bool `bson:"taskrunner_disabled"`
	RepotrackerDisabled          bool `bson:"repotracker_disabled"`
	SchedulerDisabled            bool `bson:"scheduler_disabled"`
	GithubPRTestingDisabled      bool `bson:"github_pr_testing_disabled"`
	RepotrackerPushEventDisabled bool `bson:"repotracker_push_event_disabled"`
	CLIUpdatesDisabled           bool `bson:"cli_updates_disabled"`
	GithubStatusAPIDisabled      bool `bson:"github_status_api_disabled"`
}

type eventDataNew struct {
	ResourceType string           `bson:"r_type"`
	User         string           `bson:"user"`
	Section      string           `bson:"section"`
	Changes      configDataChange `bson:"changes"`
}

type configDataChange struct {
	Before interface{} `bson:"before"`
	After  interface{} `bson:"after"`
}

type dbSettings struct {
	Banner      string `bson:"banner"`
	BannerTheme string `bson:"banner_theme"`
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
		}
		switch changeType {
		case eventTypeTheme:
			before := &dbSettings{BannerTheme: oldData.OldVal}
			after := &dbSettings{BannerTheme: oldData.NewVal}
			newData.Section = "global"
			newData.Changes = configDataChange{Before: before, After: after}
		case eventTypeBanner:
			before := &dbSettings{Banner: oldData.OldVal}
			after := &dbSettings{Banner: oldData.NewVal}
			newData.Section = "global"
			newData.Changes = configDataChange{Before: before, After: after}
		case eventTypeServiceFlags:
			before := oldData.OldFlags
			after := oldData.NewFlags
			newData.Section = "service_flags"
			newData.Changes = configDataChange{Before: &before, After: &after}
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
