package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ContainerPool holds settings for each container pool
type ContainerPool struct {
	// Distro of parent host that hosts containers
	Distro string `bson:"distro" json:"distro" yaml:"distro"`
	// ID of container pool
	Id string `bson:"id" json:"id" yaml:"id"`
	// Maximum number of containers per parent host with this container pool
	MaxContainers int `bson:"max_containers" json:"max_containers" yaml:"max_containers"`
	// Port number to start at for SSH connections
	Port uint16 `bson:"port" json:"port" yaml:"port"`
	// # of images that can be on a single host, defaults to 3 if not set
	MaxImages int
}

type ContainerPoolsConfig struct {
	Pools []ContainerPool `bson:"pools" json:"pools" yaml:"pools"`
}

func (c *ContainerPoolsConfig) SectionId() string { return "container_pools" }

func (c *ContainerPoolsConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = ContainerPoolsConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *ContainerPoolsConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			poolsKey: c.Pools,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

// GetContainerPool retrieves the container pool with a given id from
// a ContainerPoolsConfig struct
func (c *ContainerPoolsConfig) GetContainerPool(id string) *ContainerPool {
	for _, pool := range c.Pools {
		if pool.Id == id {
			return &pool
		}
	}
	return nil
}

func (c *ContainerPoolsConfig) ValidateAndDefault() error {
	// ensure that max_containers is positive
	for _, pool := range c.Pools {
		if pool.MaxContainers <= 0 {
			return errors.Errorf("container pool field max_containers must be positive integer")
		}
	}
	return nil
}
