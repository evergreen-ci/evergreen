package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
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
	// Insecure disables TLS, this should only be used for testing.
	Insecure bool `bson:"insecure" json:"insecure" yaml:"insecure"`
}

var (
	cedarConfigBaseURLKey  = bsonutil.MustHaveTag(CedarConfig{}, "BaseURL")
	cedarConfigRPCPortKey  = bsonutil.MustHaveTag(CedarConfig{}, "RPCPort")
	cedarConfigUserKey     = bsonutil.MustHaveTag(CedarConfig{}, "User")
	cedarConfigAPIKeyKey   = bsonutil.MustHaveTag(CedarConfig{}, "APIKey")
	cedarConfigInsecureKey = bsonutil.MustHaveTag(CedarConfig{}, "Insecure")
)

func (*CedarConfig) SectionId() string { return "cedar" }

func (c *CedarConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = CedarConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *CedarConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			cedarConfigBaseURLKey:  c.BaseURL,
			cedarConfigRPCPortKey:  c.RPCPort,
			cedarConfigUserKey:     c.User,
			cedarConfigAPIKeyKey:   c.APIKey,
			cedarConfigInsecureKey: c.Insecure,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *CedarConfig) ValidateAndDefault() error { return nil }
