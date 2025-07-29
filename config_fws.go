package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// FWSConfig represents the configuration of the foliage web service client.
type FWSConfig struct {
	URL string `bson:"url" json:"url" yaml:"url"`
}

var (
	fwsURLKey = bsonutil.MustHaveTag(FWSConfig{}, "URL")
)

func (c *FWSConfig) SectionId() string { return "fws" }

func (c *FWSConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *FWSConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			fwsURLKey: c.URL,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *FWSConfig) ValidateAndDefault() error {
	return nil
}
