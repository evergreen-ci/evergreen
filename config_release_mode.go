package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ReleaseModeConfig holds settings for release mode.
type ReleaseModeConfig struct {
	DistroMaxHostsFactor      float64 `bson:"distro_max_hosts_factor" json:"distro_max_hosts_factor" yaml:"distro_max_hosts_factor"`
	TargetTimeSecondsOverride int     `bson:"target_time_seconds_override" json:"target_time_seconds_override" yaml:"target_time_seconds_override"`
	IdleTimeSecondsOverride   int     `bson:"idle_time_seconds_override" json:"idle_time_seconds_override" yaml:"idle_time_seconds_override"`
}

func (c *ReleaseModeConfig) SectionId() string { return "release_mode" }

func (c *ReleaseModeConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *ReleaseModeConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"distro_max_hosts_factor":      c.DistroMaxHostsFactor,
			"target_time_seconds_override": c.TargetTimeSecondsOverride,
			"idle_time_seconds_override":   c.IdleTimeSecondsOverride,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *ReleaseModeConfig) ValidateAndDefault() error {
	if c.TargetTimeSecondsOverride < 0 {
		return errors.Errorf("target time seconds override cannot be negative")
	}
	if c.IdleTimeSecondsOverride < 0 {
		return errors.Errorf("idle time seconds override cannot be negative")
	}
	if c.DistroMaxHostsFactor < 0 {
		return errors.Errorf("distro max hosts factor cannot be negative")
	}
	if c.DistroMaxHostsFactor == 0 {
		c.DistroMaxHostsFactor = 1
	}
	return nil
}
