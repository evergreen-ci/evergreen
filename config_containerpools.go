package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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

func (c *ContainerPoolsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *ContainerPoolsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			poolsKey: c.Pools,
		}}), "updating config section '%s'", c.SectionId(),
	)
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
			return errors.Errorf("container pool max containers must be positive integer")
		}
	}
	return nil
}
