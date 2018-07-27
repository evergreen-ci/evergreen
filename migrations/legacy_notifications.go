package migrations

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const migrationLegacyNotificationsToSubscriptions = "legacy-notifications-to-subscriptions"

func legacyNotificationsToSubscriptions(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
	const (
		collection    = "project_refs"
		migrationName = "legacy-notifications-to-subscriptions"

		alertSettings = "alert-settings"
	)

	if err := env.RegisterManualMigrationOperation(migrationName, makeGithubHooksMigration(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: collection,
		},
		Limit: args.limit,
		Query: bson.M{
			"$and": []bson.M{
				{
					alertSettings: {"$ne": bson.M{}},
				},
				{
					alertSettings: {"$exists": true},
				},
			},
		},
		JobID: args.id,
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

type legacyNotificationRecipient struct {
	Recipient string `bson:"recipient"`
	Project   string `bson:"project"`
	IssueType string `bson:"issue"`
}

type legacyNotificationConfig struct {
	Provider string                      `bson:"provider"`
	Settings legacyNotificationRecipient `bson:"settings"`
}

func makeLegacyNotificationsMigration(database string) db.MigrationOperation {
	const (
		projectRefCollection = "project_ref"

		alertSettings = "alert-settings"
		identifier    = "identifier"
	)

	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		settings := map[string][]legacyNotificationConfig{}
		projectID := ""
		for _, raw := range rawD {
			switch raw.Name {
			case alertSettings:
				if err := raw.Value.Unmarshal(&settings); err != nil {
					return errors.Wrap(err, "error unmarshaling alert_settings")
				}

			case identifier:
				if err := raw.Value.Unmarshal(&projectID); err != nil {
					return errors.Wrap(err, "error unmarshaling identifier")
				}
			}
		}

		if projectID == "" {
			return errors.New("project identifier was empty")
		}
		if len(settings) == 0 {
			grip.Info("project '%s' has no config to migrate")
			return nil
		}

		for trigger, config := range settings {
			for i, recp := range config {
				if recp.Provider == "jira" {
				} else if recp.Provider == "email" {
					if recp.Settings.Recipient == "" {
						grip.Errorf("project '%s'; trigger: '%s'; index: %d has invalid email notification config", projectID, trigger, i)
						continue
					}
				}
			}
		}

		return session.DB(database).C(projectVarsCollection).UpdateId(projectVarsID,
			bson.M{
				"$unset": bson.M{
					alertSettings: 1,
				},
			})
	}
}

func legacyJIRAToSubscription(projectID, trigger string, recp legacyNotificationRecipient) error {
	if recp.Settings.Project == "" || recp.Settings.IssueType {
		return errors.New("invalid jira notification config")
	}

	event.NewJIRASubscriber(recp)

	event.Subscriber{
		ID:   bson.NewObjectId().Hex(),
		Type: "",
		Selectors: []event.Selectors{
			{
				Type: "object",
				Data: "",
			},
		},
	}
}

func oldTriggerToSubscription(projectID, trigger string, s event.Subscriber) (*event.Subscription, error) {
	subscription := event.Subscription{
		ID:         bson.NewObjectId().Hex(),
		OwnerType:  event.OwnerTypeProject,
		Owner:      projectID,
		Subscriber: s,
		Selectors: []event.Selector{
			{
				Type: "project",
				Data: projectID,
			},
			{
				Type: "requester",
				Data: evergreen.RepotrackerVersionRequester,
			},
		},
	}

	switch trigger {
	case "first_version_failure":
		subscription.Type = event.ResourceTypeTask
		subscription.Trigger = "first-failure-in-version"

	case "first_variant_failure":
		subscription.Type = event.ResourceTypeTask
		subscription.Trigger = "first-failure-in-build"

	case "first_tasktype_failure":
		subscription.Type = event.ResourceTypeTask
		subscription.Trigger = "first-failure-in-version-with-name"

	case "task_transition_failure":
		subscription.Type = event.ResourceTypeTask
		subscription.Trigger = "task-regression"

	default:
		return nil, errors.Errorf("unknown trigger %s", trigger)
	}

	return &subscription, nil
}
