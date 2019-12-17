package evergreen

import (
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AmboyConfig struct {
	Name                                  string `bson:"name" json:"name" yaml:"name"`
	SingleName                            string `bson:"single_name" json:"single_name" yaml:"single_name"`
	DB                                    string `bson:"database" json:"database" yaml:"database"`
	PoolSizeLocal                         int    `bson:"pool_size_local" json:"pool_size_local" yaml:"pool_size_local"`
	PoolSizeRemote                        int    `bson:"pool_size_remote" json:"pool_size_remote" yaml:"pool_size_remote"`
	LocalStorage                          int    `bson:"local_storage_size" json:"local_storage_size" yaml:"local_storage_size"`
	GroupDefaultWorkers                   int    `bson:"group_default_workers" json:"group_default_workers" yaml:"group_default_workers"`
	GroupBackgroundCreateFrequencyMinutes int    `bson:"group_background_create_frequency" json:"group_background_create_frequency" yaml:"group_background_create_frequency"`
	GroupPruneFrequencyMinutes            int    `bson:"group_prune_frequency" json:"group_prune_frequency" yaml:"group_prune_frequency"`
	GroupTTLMinutes                       int    `bson:"group_ttl" json:"group_ttl" yaml:"group_ttl"`
}

func (c *AmboyConfig) SectionId() string { return "amboy" }

func (c *AmboyConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	grip.Info(res.Err())
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = AmboyConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}
	return nil
}

func (c *AmboyConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)
	t := true

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"name":                              c.Name,
			"single_name":                       c.SingleName,
			"database":                          c.DB,
			"pool_size_local":                   c.PoolSizeLocal,
			"pool_size_remote":                  c.PoolSizeRemote,
			"local_storage_size":                c.LocalStorage,
			"group_default_workers":             c.GroupDefaultWorkers,
			"group_background_create_frequency": c.GroupBackgroundCreateFrequencyMinutes,
			"group_prune_frequency":             c.GroupPruneFrequencyMinutes,
			"group_ttl":                         c.GroupTTLMinutes,
		},
	}, &options.UpdateOptions{Upsert: &t})

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

	if c.GroupDefaultWorkers <= 0 {
		c.GroupDefaultWorkers = defaultGroupWorkers
	}

	if c.GroupBackgroundCreateFrequencyMinutes <= 0 {
		c.GroupBackgroundCreateFrequencyMinutes = defaultGroupBackgroundCreateFrequencyMinutes
	}

	if c.GroupPruneFrequencyMinutes <= 0 {
		c.GroupPruneFrequencyMinutes = defaultGroupPruneFrequencyMinutes
	}

	if c.GroupTTLMinutes <= 0 {
		c.GroupTTLMinutes = defaultGroupTTLMinutes
	}

	return nil
}
