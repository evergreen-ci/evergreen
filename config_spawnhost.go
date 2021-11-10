package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SpawnHostConfig struct {
	UnexpirableHostsPerUser   int `yaml:"unexpirable_hosts_per_user" bson:"unexpirable_hosts_per_user" json:"unexpirable_hosts_per_user"`
	UnexpirableVolumesPerUser int `yaml:"unexpirable_volumes_per_user" bson:"unexpirable_volumes_per_user" json:"unexpirable_volumes_per_user"`
	SpawnHostsPerUser         int `yaml:"spawn_hosts_per_user" bson:"spawn_hosts_per_user" json:"spawn_hosts_per_user"`
}

func (c *SpawnHostConfig) SectionId() string { return "spawnhost" }

func (c *SpawnHostConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)
	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = SpawnHostConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *SpawnHostConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			unexpirableHostsPerUserKey:   c.UnexpirableHostsPerUser,
			unexpirableVolumesPerUserKey: c.UnexpirableVolumesPerUser,
			spawnhostsPerUserKey:         c.SpawnHostsPerUser,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
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
