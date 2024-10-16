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
	Insecure     bool   `bson:"insecure" json:"insecure" yaml:"insecure"`
	SendToCedar  bool   `bson:"send_to_cedar" json:"send_to_cedar" yaml:"send_to_cedar"`
	SPSURL       string `bson:"sps_url" json:"sps_url" yaml:"sps_url"`
	SendRatioSPS int    `bson:"send_ratio_sps" json:"send_ratio_sps" yaml:"send_ratio_sps"`
}

var (
	cedarConfigBaseURLKey     = bsonutil.MustHaveTag(CedarConfig{}, "BaseURL")
	cedarConfigGRPCBaseURLKey = bsonutil.MustHaveTag(CedarConfig{}, "GRPCBaseURL")
	cedarConfigRPCPortKey     = bsonutil.MustHaveTag(CedarConfig{}, "RPCPort")
	cedarConfigUserKey        = bsonutil.MustHaveTag(CedarConfig{}, "User")
	cedarConfigAPIKeyKey      = bsonutil.MustHaveTag(CedarConfig{}, "APIKey")
	cedarConfigInsecureKey    = bsonutil.MustHaveTag(CedarConfig{}, "Insecure")
	cedarSendToCedarKey       = bsonutil.MustHaveTag(CedarConfig{}, "SendToCedar")
	cedarSPSURLKey            = bsonutil.MustHaveTag(CedarConfig{}, "SPSURL")
	cedarSendRatioSPSKey      = bsonutil.MustHaveTag(CedarConfig{}, "SendRatioSPS")
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
			cedarSendToCedarKey:       c.SendToCedar,
			cedarSPSURLKey:            c.SPSURL,
			cedarSendRatioSPSKey:      c.SendRatioSPS,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *CedarConfig) ValidateAndDefault() error { return nil }
