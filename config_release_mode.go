package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ReleaseModeConfig holds settings for release mode.
type ReleaseModeConfig struct {
	DistroMaxHostsFactor float64 `bson:"distro_max_hosts_factor" json:"distro_max_hosts_factor" yaml:"distro_max_hosts_factor"`
	TargetTimeOverride   int     `bson:"target_time_override" json:"target_time_override" yaml:"target_time_override"`
}

func (c *ReleaseModeConfig) SectionId() string { return "release_mode" }

func (c *ReleaseModeConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *ReleaseModeConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"distro_max_hosts_factor": c.DistroMaxHostsFactor,
			"target_time_override":    c.TargetTimeOverride,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *ReleaseModeConfig) ValidateAndDefault() error { return nil }
