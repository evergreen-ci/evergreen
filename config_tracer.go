package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// TracerConfig configures the OpenTelemetry tracer provider. If not enabled traces will not be sent.
type TracerConfig struct {
	Enabled                   bool   `yaml:"enabled" bson:"enabled" json:"enabled"`
	CollectorEndpoint         string `yaml:"collector_endpoint" bson:"collector_endpoint" json:"collector_endpoint"`
	CollectorAPIKey           string `yaml:"collector_api_key" bson:"collector_api_key" json:"collector_api_key"`
	CollectorInternalEndpoint string `yaml:"collector_internal_endpoint" bson:"collector_internal_endpoint" json:"collector_internal_endpoint"`
}

// SectionId returns the ID of this config section.
func (c *TracerConfig) SectionId() string { return "tracer" }

// Get populates the config from the database.
func (c *TracerConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

// Set sets the document in the database to match the in-memory config struct.
func (c *TracerConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			tracerEnabledKey:                   c.Enabled,
			tracerCollectorEndpointKey:         c.CollectorEndpoint,
			tracerCollectorInternalEndpointKey: c.CollectorInternalEndpoint,
			tracerCollectorAPIKeyKey:           c.CollectorAPIKey,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

// ValidateAndDefault validates the tracer configuration.
func (c *TracerConfig) ValidateAndDefault() error {
	if c.Enabled && c.CollectorEndpoint == "" {
		return errors.New("tracer can't be enabled without a collector endpoint")
	}
	return nil
}
