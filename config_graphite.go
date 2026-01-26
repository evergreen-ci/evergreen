package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GraphiteConfig stores configuration for Graphite integration.
type GraphiteConfig struct {
	CIOptimizationToken string `bson:"ci_optimization_token" json:"ci_optimization_token" yaml:"ci_optimization_token" secret:"true"`
	ServerURL           string `bson:"server_url" json:"server_url" yaml:"server_url"`
}

func (c *GraphiteConfig) SectionId() string { return "graphite" }

func (c *GraphiteConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *GraphiteConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			graphiteCIOptimizationTokenKey: c.CIOptimizationToken,
			graphiteServerURLKey:           c.ServerURL,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *GraphiteConfig) ValidateAndDefault() error {
	if c.CIOptimizationToken != "" && c.ServerURL == "" {
		return errors.New("must specify a Graphite server URL if a token is specified")
	}
	return nil
}
