package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type CedarConfig struct {
	BaseURL     string `bson:"base_url" json:"base_url" yaml:"base_url"`
	GRPCBaseURL string `bson:"grpc_base_url" json:"grpc_base_url" yaml:"grpc_base_url"`
	RPCPort     string `bson:"rpc_port" json:"rpc_port" yaml:"rpc_port"`
	User        string `bson:"user" json:"user" yaml:"user"`
	APIKey      string `bson:"api_key" json:"api_key" yaml:"api_key"`
	// Insecure disables TLS, this should only be used for testing.
	Insecure bool `bson:"insecure" json:"insecure" yaml:"insecure"`
}

var (
	cedarConfigBaseURLKey     = bsonutil.MustHaveTag(CedarConfig{}, "BaseURL")
	cedarConfigGRPCBaseURLKey = bsonutil.MustHaveTag(CedarConfig{}, "GRPCBaseURL")
	cedarConfigRPCPortKey     = bsonutil.MustHaveTag(CedarConfig{}, "RPCPort")
	cedarConfigUserKey        = bsonutil.MustHaveTag(CedarConfig{}, "User")
	cedarConfigAPIKeyKey      = bsonutil.MustHaveTag(CedarConfig{}, "APIKey")
	cedarConfigInsecureKey    = bsonutil.MustHaveTag(CedarConfig{}, "Insecure")
)

func (*CedarConfig) SectionId() string { return "cedar" }

func (c *CedarConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *CedarConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			cedarConfigBaseURLKey:     c.BaseURL,
			cedarConfigGRPCBaseURLKey: c.GRPCBaseURL,
			cedarConfigRPCPortKey:     c.RPCPort,
			cedarConfigUserKey:        c.User,
			cedarConfigAPIKeyKey:      c.APIKey,
			cedarConfigInsecureKey:    c.Insecure,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *CedarConfig) ValidateAndDefault() error { return nil }
