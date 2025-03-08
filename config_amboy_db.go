package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// AmboyDBConfig configures Amboy's database connection.
type AmboyDBConfig struct {
	URL      string `bson:"url" json:"url" yaml:"url"`
	Database string `bson:"database" json:"database" yaml:"database"`
}

func (c *AmboyDBConfig) SectionId() string { return "amboy_db" }

var (
	amboyDBURLKey      = bsonutil.MustHaveTag(AmboyDBConfig{}, "URL")
	amboyDBDatabaseKey = bsonutil.MustHaveTag(AmboyDBConfig{}, "Database")
)

func (c *AmboyDBConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *AmboyDBConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			amboyDBURLKey:      c.URL,
			amboyDBDatabaseKey: c.Database,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *AmboyDBConfig) ValidateAndDefault() error {
	if c.Database == "" {
		c.Database = defaultAmboyDBName
	}
	return nil
}
