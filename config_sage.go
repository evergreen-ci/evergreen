package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// SageConfig holds configuration for the Sage API service.
type SageConfig struct {
	BaseURL string `bson:"base_url" json:"base_url" yaml:"base_url"`
}

var (
	sageBaseURLKey = bsonutil.MustHaveTag(SageConfig{}, "BaseURL")
)

func (*SageConfig) SectionId() string { return "sage" }

func (c *SageConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *SageConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			sageBaseURLKey: c.BaseURL,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *SageConfig) ValidateAndDefault() error { return nil }
