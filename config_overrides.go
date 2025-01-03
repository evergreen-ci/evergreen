package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const overridesSectionID = "overrides"

// AmboyDBConfig configures Amboy's database connection.
type OverridesConfig struct {
	Overrides bson.M `bson:"overrides" json:"overrides" yaml:"overrides"`
}

func (c *OverridesConfig) SectionId() string { return overridesSectionID }

var (
	overridesKey = bsonutil.MustHaveTag(OverridesConfig{}, "Overrides")
)

func (c *OverridesConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *OverridesConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			overridesKey: c.Overrides,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *OverridesConfig) ValidateAndDefault() error {
	return nil
}
