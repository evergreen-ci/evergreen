package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ParameterStoreConfig stores configuration for using SSM Parameter Store.
type ParameterStoreConfig struct {
	// Prefix is the Parameter Store path prefix for the Evergreen application.
	Prefix string `bson:"prefix" json:"prefix" yaml:"prefix"`
}

var (
	prefixKey = bsonutil.MustHaveTag(ParameterStoreConfig{}, "Prefix")
)

func (*ParameterStoreConfig) SectionId() string { return "parameter_store" }

func (c *ParameterStoreConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *ParameterStoreConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			prefixKey: c.Prefix,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *ParameterStoreConfig) ValidateAndDefault() error {
	return nil
}
