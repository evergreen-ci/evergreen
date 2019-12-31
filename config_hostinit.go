package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	SSHTimeoutSeconds int64 `bson:"ssh_timeout_secs" json:"ssh_timeout_secs" yaml:"sshtimeoutseconds"`
}

func (c *HostInitConfig) SectionId() string { return "hostinit" }

func (c *HostInitConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = HostInitConfig{}
			return nil
		}

		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *HostInitConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"ssh_timeout_secs": c.SSHTimeoutSeconds,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *HostInitConfig) ValidateAndDefault() error { return nil }
