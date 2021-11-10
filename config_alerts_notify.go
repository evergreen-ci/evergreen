package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DefaultEventProcessingLimit    = 1000
	DefaultBufferIntervalSeconds   = 60
	DefaultBufferTargetPerInterval = 20
)

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	BufferTargetPerInterval int        `bson:"buffer_target_per_interval" json:"buffer_target_per_interval" yaml:"buffer_target_per_interval"`
	BufferIntervalSeconds   int        `bson:"buffer_interval_seconds" json:"buffer_interval_seconds" yaml:"buffer_interval_seconds"`
	EventProcessingLimit    int        `bson:"event_processing_limit" json:"event_processing_limit" yaml:"event_processing_limit"`
	SMTP                    SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
}

func (c *NotifyConfig) SectionId() string { return "notify" }

func (c *NotifyConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = NotifyConfig{}
			return nil
		}

		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *NotifyConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.ReplaceOne(ctx, byId(c.SectionId()), c, options.Replace().SetUpsert(true))
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *NotifyConfig) ValidateAndDefault() error {
	if c.BufferIntervalSeconds <= 0 {
		c.BufferIntervalSeconds = DefaultBufferIntervalSeconds
	}
	if c.BufferTargetPerInterval <= 0 {
		c.BufferTargetPerInterval = DefaultBufferTargetPerInterval
	}

	// cap to 100 jobs/sec per server
	jobsPerSecond := c.BufferIntervalSeconds / c.BufferTargetPerInterval
	if jobsPerSecond > maxNotificationsPerSecond {
		return errors.Errorf("maximum notification jobs per second is %d", maxNotificationsPerSecond)

	}

	if c.EventProcessingLimit <= 0 {
		c.EventProcessingLimit = DefaultEventProcessingLimit
	}

	return nil
}

type AlertsConfig struct {
	SMTP SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
}

func (c *AlertsConfig) SectionId() string { return "alerts" }

func (c *AlertsConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = AlertsConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *AlertsConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"smtp": c.SMTP,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *AlertsConfig) ValidateAndDefault() error { return nil }

// SMTPConfig holds SMTP email settings.
type SMTPConfig struct {
	Server     string   `bson:"server" json:"server" yaml:"server"`
	Port       int      `bson:"port" json:"port" yaml:"port"`
	UseSSL     bool     `bson:"use_ssl" json:"use_ssl" yaml:"use_ssl"`
	Username   string   `bson:"username" json:"username" yaml:"username"`
	Password   string   `bson:"password" json:"password" yaml:"password"`
	From       string   `bson:"from" json:"from" yaml:"from"`
	AdminEmail []string `bson:"admin_email" json:"admin_email" yaml:"admin_email"`
}
