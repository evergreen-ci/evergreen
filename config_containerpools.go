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

<<<<<<< HEAD
// GetContainerPool retrieves the container pool with a given id from
// a ContainerPoolsConfig struct
=======
>>>>>>> EVG-3520 add pool mapping to admin collection and methods to get/set mapping
func (c *ContainerPoolsConfig) GetContainerPool(id string) (ContainerPool, error) {
	for _, pool := range c.Pools {
		if pool.Id == id {
			return pool, nil
		}
	}
	return ContainerPool{}, errors.Errorf("error retrieving container pool %s", id)
}

func (c *ContainerPoolsConfig) ValidateAndDefault() error { return nil }
