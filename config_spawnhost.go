package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type SpawnHostConfig struct {
	UnexpirableHostsPerUser   int `yaml:"unexpirable_hosts_per_user" bson:"unexpirable_hosts_per_user" json:"unexpirable_hosts_per_user"`
	UnexpirableVolumesPerUser int `yaml:"unexpirable_volumes_per_user" bson:"unexpirable_volumes_per_user" json:"unexpirable_volumes_per_user"`
	SpawnHostsPerUser         int `yaml:"spawn_hosts_per_user" bson:"spawn_hosts_per_user" json:"spawn_hosts_per_user"`
}

func (c *SpawnHostConfig) SectionId() string { return "spawnhost" }

func (c *SpawnHostConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *SpawnHostConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			unexpirableHostsPerUserKey:   c.UnexpirableHostsPerUser,
			unexpirableVolumesPerUserKey: c.UnexpirableVolumesPerUser,
			spawnhostsPerUserKey:         c.SpawnHostsPerUser,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *SpawnHostConfig) ValidateAndDefault() error {
	if c.SpawnHostsPerUser < 0 {
		c.SpawnHostsPerUser = DefaultMaxSpawnHostsPerUser
	}
	if c.UnexpirableHostsPerUser < 0 {
		c.UnexpirableHostsPerUser = DefaultUnexpirableHostsPerUser
	}
	if c.UnexpirableVolumesPerUser < 0 {
		c.UnexpirableVolumesPerUser = DefaultUnexpirableVolumesPerUser
	}
	return nil
}
