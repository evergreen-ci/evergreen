package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const overridesSectionID = "overrides"

// OverridesConfig overrides individual fields from other configuration documents.
type OverridesConfig struct {
	Overrides []Override `bson:"overrides" json:"overrides" yaml:"overrides"`
}

type Override struct {
	// SectionID is the ID of the section being overridden.
	SectionID string `bson:"section_id" json:"section_id" yaml:"section_id"`
	// Field is the name of the field being overridden. A nested field is indicated with dot notation.
	Field string `bson:"field" json:"field" yaml:"field"`
	// Value is the new value to set the field to.
	Value interface{} `bson:"value" json:"value" yaml:"value"`
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
	catcher := grip.NewBasicCatcher()
	for _, override := range c.Overrides {
		catcher.Add(override.validate())
	}
	return catcher.Resolve()
}

func (o *Override) validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.AddWhen(o.SectionID == "", errors.New("section ID can't be empty"))
	catcher.AddWhen(o.Field == "", errors.New("field name can't be empty"))
	catcher.AddWhen(o.Value == nil, errors.New("value can't be empty"))
	return catcher.Resolve()
}

// sectionOverrides returns all overrides relevant to the given section ID.
func (c *OverridesConfig) sectionOverrides(sectionID string) []Override {
	var overrides []Override
	for _, override := range c.Overrides {
		if override.SectionID == sectionID {
			overrides = append(overrides, override)
		}
	}
	return overrides
}
