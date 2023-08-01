package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DefaultBufferIntervalSeconds   = 60
	DefaultBufferTargetPerInterval = 20
)

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	BufferTargetPerInterval int       `bson:"buffer_target_per_interval" json:"buffer_target_per_interval" yaml:"buffer_target_per_interval"`
	BufferIntervalSeconds   int       `bson:"buffer_interval_seconds" json:"buffer_interval_seconds" yaml:"buffer_interval_seconds"`
	SES                     SESConfig `bson:"ses" json:"ses" yaml:"ses"`
}

func (c *NotifyConfig) SectionId() string { return "notify" }

func (c *NotifyConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = NotifyConfig{}
			return nil
		}

		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *NotifyConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).ReplaceOne(ctx, byId(c.SectionId()), c, options.Replace().SetUpsert(true))
	return errors.Wrapf(err, "updating section '%s'", c.SectionId())
}

func (c *NotifyConfig) ValidateAndDefault() error {
	if c.BufferIntervalSeconds <= 0 {
		c.BufferIntervalSeconds = DefaultBufferIntervalSeconds
	}
	if c.BufferTargetPerInterval <= 0 {
		c.BufferTargetPerInterval = DefaultBufferTargetPerInterval
	}

	// Cap to 100 jobs/sec per server.
	jobsPerSecond := c.BufferIntervalSeconds / c.BufferTargetPerInterval
	if jobsPerSecond > maxNotificationsPerSecond {
		return errors.Errorf("maximum notification jobs per second is %d", maxNotificationsPerSecond)

	}

	return nil
}

// SESConfig configures the SES email sender.
type SESConfig struct {
	SenderAddress string `bson:"sender_address" json:"sender_address" yaml:"sender_address"`
}
