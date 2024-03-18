package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	sleepSchedulePermanentlyExemptHostsKey = bsonutil.MustHaveTag(SleepScheduleConfig{}, "PermanentlyExemptHosts")
)

// SleepScheduleConfig holds relevant settings for unexpirable host sleep
// schedules.
type SleepScheduleConfig struct {
	PermanentlyExemptHosts []string `bson:"permanently_exempt_hosts" json:"permanently_exempt_hosts"`
}

func (c *SleepScheduleConfig) SectionId() string { return "sleep_schedule" }

func (c *SleepScheduleConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = SleepScheduleConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *SleepScheduleConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			sleepSchedulePermanentlyExemptHostsKey: c.PermanentlyExemptHosts,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *SleepScheduleConfig) ValidateAndDefault() error {
	return nil
}
