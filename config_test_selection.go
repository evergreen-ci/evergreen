package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// TestSelectionConfig represents the configuration of the test selection
// service.
type TestSelectionConfig struct {
	URL string `bson:"url" json:"url" yaml:"url"`
}

var (
	testSelectionURLKey = bsonutil.MustHaveTag(TestSelectionConfig{}, "URL")
)

func (c *TestSelectionConfig) SectionId() string { return "test_selection" }

func (c *TestSelectionConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *TestSelectionConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			testSelectionURLKey: c.URL,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *TestSelectionConfig) ValidateAndDefault() error {
	return nil
}
