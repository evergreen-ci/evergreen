package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultHostThrottle = 32

// HostInitConfig holds logging settings for the hostinit process.
type HostInitConfig struct {
	HostThrottle         int `bson:"host_throttle" json:"host_throttle" yaml:"host_throttle"`
	ProvisioningThrottle int `bson:"provisioning_throttle" json:"provisioning_throttle" yaml:"provisioning_throttle"`
	CloudStatusBatchSize int `bson:"cloud_batch_size" json:"cloud_batch_size" yaml:"cloud_batch_size"`
}

func (c *HostInitConfig) SectionId() string { return "hostinit" }

func (c *HostInitConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = HostInitConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	if err := res.Decode(c); err != nil {
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
			"host_throttle":         c.HostThrottle,
			"provisioning_throttle": c.ProvisioningThrottle,
			"cloud_batch_size":      c.CloudStatusBatchSize,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *HostInitConfig) ValidateAndDefault() error {
	if c.HostThrottle <= 0 {
		c.HostThrottle = defaultHostThrottle
	}

	if c.ProvisioningThrottle <= 0 {
		c.ProvisioningThrottle = 200
	}

	if c.CloudStatusBatchSize <= 0 {
		c.CloudStatusBatchSize = 500
	}

	return nil
}
