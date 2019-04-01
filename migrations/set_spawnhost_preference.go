package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	userCollection                         = "users"
	subscriptionCollection                 = "subscriptions"
	migrationSpawnhostExpirationPreference = "spawnhost-expiration-preference"
)

type emailSubscription struct {
	ID             mgobson.ObjectId `bson:"_id"`
	Subscriber     emailSubscriber     `bson:"subscriber"`
	Owner          string              `bson:"owner"`
	OwnerType      string              `bson:"owner_type"`
	TriggerData    interface{}         `bson:"trigger_data"`
	Type           string              `bson:"type"`
	Trigger        string              `bson:"trigger"`
	Selectors      []emailSelector     `bson:"selectors"`
	RegexSelectors []interface{}       `bson:"regex_selectors"`
}

type emailSubscriber struct {
	Type   string `bson:"type"`
	Target string `bson:"target"`
}

type emailSelector struct {
	Type string `bson:"type"`
	Data string `bson:"data"`
}

type user struct {
	ID       string       `bson:"_id"`
	Email    string       `bson:"email"`
	Settings userSettings `bson:"settings"`
}

type userSettings struct {
	Notifications notifications `bson:"notifications"`
}

type notifications struct {
	SpawnHostExpiration   string              `bson:"spawn_host_expiration"`
	SpawnHostExpirationID mgobson.ObjectId `bson:"spawn_host_expiration_id"`
}

func setSpawnhostPreferenceGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {

	if err := env.RegisterManualMigrationOperation(migrationSpawnhostExpirationPreference, makeSpawnhostExpirationPreferenceMigration(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: userCollection,
		},
		JobID: args.id,
		Limit: args.limit,
		Query: db.Document{},
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationSpawnhostExpirationPreference), nil
}

func makeSpawnhostExpirationPreferenceMigration(database string) db.MigrationOperation {
	const (
		idKey             = "_id"
		preferenceKey     = "spawn_host_expiration"
		subscriptionIdKey = "spawn_host_expiration_id"
		settingsKey       = "settings"
		notificationsKey  = "notifications"
		emailKey          = "email"
	)
	return func(session db.Session, rawD mgobson.RawD) error {
		defer session.Close()

		var userId string
		var userSettingsVal userSettings
		var email string
		for _, raw := range rawD {
			switch raw.Name {
			case idKey:
				if err := raw.Value.Unmarshal(&userId); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}
			case settingsKey:
				if err := raw.Value.Unmarshal(&userSettingsVal); err != nil {
					return errors.Wrap(err, "error unmarshaling settings")
				}
			case emailKey:
				if err := raw.Value.Unmarshal(&email); err != nil {
					return errors.Wrap(err, "error unmarshaling email")
				}
			}
		}
		// don't do anything if there's a preference already
		if userSettingsVal.Notifications.SpawnHostExpiration != "" || userSettingsVal.Notifications.SpawnHostExpirationID.Valid() {
			return nil
		}

		sub := makeSubscription(userId, email)
		if err := session.DB(database).C(subscriptionCollection).Insert(sub); err != nil {
			return errors.Wrap(err, "error creating subscription")
		}

		update := db.Document{
			"$set": db.Document{
				bsonutil.GetDottedKeyName(settingsKey, notificationsKey, preferenceKey):     "email",
				bsonutil.GetDottedKeyName(settingsKey, notificationsKey, subscriptionIdKey): sub.ID,
			},
		}
		return session.DB(database).C(userCollection).UpdateId(userId, update)
	}
}

func makeSubscription(user, email string) emailSubscription {
	return emailSubscription{
		ID:        mgobson.NewObjectId(),
		Owner:     user,
		OwnerType: "person",
		Type:      "HOST",
		Trigger:   "expiration",
		Subscriber: emailSubscriber{
			Type:   "email",
			Target: email,
		},
		Selectors: []emailSelector{
			{Type: "owner", Data: user},
		},
	}
}
