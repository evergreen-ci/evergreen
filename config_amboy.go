package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type AmboyConfig struct {
	Name           string `bson:"name" json:"name" yaml:"name"`
	SingleName     string `bson:"single_name" json:"single_name" yaml:"single_name"`
	DB             string `bson:"database" json:"database" yaml:"database"`
	PoolSizeLocal  int    `bson:"pool_size_local" json:"pool_size_local" yaml:"pool_size_local"`
	PoolSizeRemote int    `bson:"pool_size_remote" json:"pool_size_remote" yaml:"pool_size_remote"`
	LocalStorage   int    `bson:"local_storage_size" json:"local_storage_size" yaml:"local_storage_size"`
}

func (c *AmboyConfig) SectionId() string { return "amboy" }

func (c *AmboyConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = AmboyConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *AmboyConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"name":               c.Name,
			"single_name":        c.SingleName,
			"database":           c.DB,
			"pool_size_local":    c.PoolSizeLocal,
			"pool_size_remote":   c.PoolSizeRemote,
			"local_storage_size": c.LocalStorage,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *AmboyConfig) ValidateAndDefault() error {
	if c.Name == "" {
		c.Name = defaultAmboyQueueName
	}

	if c.SingleName == "" {
		c.SingleName = defaultSingleAmboyQueueName
	}

	if c.DB == "" {
		c.DB = defaultAmboyDBName
	}

	if c.PoolSizeLocal == 0 {
		c.PoolSizeLocal = defaultAmboyPoolSize
	}

	if c.PoolSizeRemote == 0 {
		c.PoolSizeRemote = defaultAmboyPoolSize
	}

	if c.LocalStorage == 0 {
		c.LocalStorage = defaultAmboyLocalStorageSize
	}

	return nil
}
