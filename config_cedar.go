package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CedarConfig struct {
	BaseURL string `bson:"base_url" json:"base_url" yaml:"base_url"`
	RPCPort string `bson:"rpc_port" json:"rpc_port" yaml:"rpc_port"`
	User    string `bson:"user" json:"user" yaml:"user"`
	APIKey  string `bson:"api_key" json:"api_key" yaml:"api_key"`
}

func (*CedarConfig) SectionId() string { return "cedar" }

func (c *CedarConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = CedarConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *CedarConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"base_url": c.BaseURL,
			"rpc_port": c.RPCPort,
			"user":     c.User,
			"api_key":  c.APIKey,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *CedarConfig) ValidateAndDefault() error { return nil }
