package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	return getConfigSection(ctx, c)
}

func (c *SleepScheduleConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			sleepSchedulePermanentlyExemptHostsKey: c.PermanentlyExemptHosts,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *SleepScheduleConfig) ValidateAndDefault() error {
	return nil
}
