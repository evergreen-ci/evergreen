package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GraphiteConfig stores configuration for Graphite integration.
type GraphiteConfig struct {
	CLIOptimizationToken string `bson:"cli_optimization_token" json:"cli_optimization_token" yaml:"cli_optimization_token" secret:"true"`
	ServerURL            string `bson:"server_url" json:"server_url" yaml:"server_url"`
}

func (c *GraphiteConfig) SectionId() string { return "graphite" }

func (c *GraphiteConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *GraphiteConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			graphiteCLIOptimizationTokenKey: c.CLIOptimizationToken,
			graphiteServerURLKey:            c.ServerURL,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *GraphiteConfig) ValidateAndDefault() error {
	if c.CLIOptimizationToken != "" && c.ServerURL == "" {
		return errors.New("must specify a Graphite server URL if a token is specified")
	}
	return nil
}
