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
	Overrides []Override `bson:"overrides" json:"overrides" yaml:"overrides"`
}

type Override struct {
	SectionID string      `bson:"section_id" json:"section_id" yaml:"section_id"`
	Field     string      `bson:"field" json:"field" yaml:"field"`
	Value     interface{} `bson:"value" json:"value" yaml:"value"`
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

func (c *OverridesConfig) sectionOverrides(sectionID string) []Override {
	var overrides []Override
	for _, override := range c.Overrides {
		if override.SectionID == sectionID {
			overrides = append(overrides, override)
		}
	}
	return overrides
}
