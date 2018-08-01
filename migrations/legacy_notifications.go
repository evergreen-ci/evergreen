package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const migrationLegacyNotificationsToSubscriptions = "legacy-notifications-to-subscriptions"

func legacyNotificationsToSubscriptionsGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
	const (
		collection    = "project_ref"
		migrationName = "legacy-notifications-to-subscriptions"

		alertSettings = "alert_settings"
	)

	if err := env.RegisterManualMigrationOperation(migrationName, makeLegacyNotificationsMigration(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: collection,
		},
		Limit: args.limit,
		Query: db.Document{
			alertSettings: db.Document{"$exists": true},
		},
		JobID: args.id,
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

type legacyNotificationSettings struct {
	Recipient string `bson:"recipient"`
	Project   string `bson:"project"`
	IssueType string `bson:"issue"`
}

type legacyNotification struct {
	Provider string                     `bson:"provider"`
	Settings legacyNotificationSettings `bson:"settings"`
}

func makeLegacyNotificationsMigration(database string) db.MigrationOperation {
	const (
		projectRefCollection = "project_ref"

		alertSettingsKey = "alert_settings"
		identifierKey    = "identifier"
	)

	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		settings := map[string][]legacyNotification{}
		projectID := ""
		for _, raw := range rawD {
			switch raw.Name {
			case alertSettingsKey:
				if err := raw.Value.Unmarshal(&settings); err != nil {
					return errors.Wrap(err, "error unmarshaling alert_settings")
				}

			case identifierKey:
				if err := raw.Value.Unmarshal(&projectID); err != nil {
					return errors.Wrap(err, "error unmarshaling identifier")
				}
			}
		}

		if projectID == "" {
			return errors.New("project identifier was empty")
		}
		if len(settings) != 0 {
			err := migrateLegacyNotifications(session.DB(database), projectID, settings)
			if err != nil {
				return err
			}
		}

		return session.DB(database).C(projectRefCollection).Update(
			db.Document{
				identifierKey: projectID,
			},
			db.Document{
				"$unset": db.Document{
					alertSettingsKey: 1,
				},
			})
	}
}

func migrateLegacyNotifications(dbSess db.Database, projectID string, settings map[string][]legacyNotification) error {
	const (
		subscriptionsCollection = "subscriptions"
	)
	catcher := grip.NewSimpleCatcher()
	subsCollection := dbSess.C(subscriptionsCollection)
	for trigger, config := range settings {
		for i, recp := range config {
			var sub *subscription
			var err error
			if recp.Provider == "jira" {
				sub, err = legacyJIRAToSubscription(projectID, trigger, recp)
			} else if recp.Provider == "email" {
				sub, err = legacyEmailToSubscription(projectID, trigger, recp)
			}
			if err != nil {
				catcher.Add(errors.Wrapf(err, "%s, index %d", trigger, i))
				continue
			}
			if err = subsCollection.Insert(sub); err != nil {
				catcher.Add(err)
			}
		}
	}

	return catcher.Resolve()

}

type subscriber struct {
	Type   string      `bson:"type"`
	Target interface{} `bson:"target"`
}

func newJIRASubscriber(project, issueType string) subscriber {
	return subscriber{
		Type: "jira-issue",
		Target: db.Document{
			"project":    project,
			"issue_type": issueType,
		},
	}
}

func newEmailSubscriber(t string) subscriber {
	return subscriber{
		Type:   "email",
		Target: t,
	}
}

type selector struct {
	Type string `bson:"type"`
	Data string `bson:"data"`
}

type subscription struct {
	ID         string     `bson:"_id"`
	Type       string     `bson:"type"`
	Trigger    string     `bson:"trigger"`
	Selectors  []selector `bson:"selectors,omitempty"`
	Subscriber subscriber `bson:"subscriber"`
	OwnerType  string     `bson:"owner_type"`
	Owner      string     `bson:"owner"`
}

func oldTriggerToSubscription(projectID, trigger string, sub subscriber) (*subscription, error) {
	s := subscription{
		ID:        bson.NewObjectId().Hex(),
		OwnerType: "project",
		Owner:     projectID,
		Selectors: []selector{
			{
				Type: "project",
				Data: projectID,
			},
			{
				Type: "requester",
				Data: "gitter_request",
			},
		},
		Subscriber: sub,
	}

	switch trigger {
	case "task_failed":
		s.Type = "TASK"
		s.Trigger = "failure"

	case "first_version_failure":
		s.Type = "TASK"
		s.Trigger = "first-failure-in-version"

	case "first_variant_failure":
		s.Type = "TASK"
		s.Trigger = "first-failure-in-build"

	case "first_tasktype_failure":
		s.Type = "TASK"
		s.Trigger = "first-failure-in-version-with-name"

	case "task_transition_failure":
		s.Type = "TASK"
		s.Trigger = "regression"

	default:
		return nil, errors.New("unknown trigger")
	}

	return &s, nil
}

func legacyJIRAToSubscription(projectID, trigger string, recp legacyNotification) (*subscription, error) {
	if recp.Settings.Project == "" || recp.Settings.IssueType == "" {
		return nil, errors.New("invalid jira notification config")
	}

	subscriber := newJIRASubscriber(recp.Settings.Project, recp.Settings.IssueType)
	s, err := oldTriggerToSubscription(projectID, trigger, subscriber)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func legacyEmailToSubscription(projectID, trigger string, recp legacyNotification) (*subscription, error) {
	if recp.Settings.Recipient == "" {
		return nil, errors.New("invalid email notification config")
	}

	subscriber := newEmailSubscriber(recp.Settings.Recipient)
	s, err := oldTriggerToSubscription(projectID, trigger, subscriber)
	if err != nil {
		return nil, err
	}

	return s, nil
}
