package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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
}

type ContainerPoolsConfig struct {
	Pools []ContainerPool `bson:"pools" json:"pools" yaml:"pools"`
}

func (c *ContainerPoolsConfig) SectionId() string { return "container_pools" }

func (c *ContainerPoolsConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = ContainerPoolsConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *ContainerPoolsConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			poolsKey: c.Pools,
		},
	})
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
