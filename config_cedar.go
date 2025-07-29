package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type CedarConfig struct {
	DBURL  string `bson:"db_url" json:"db_url" yaml:"db_url"`
	DBName string `bson:"db_name" json:"db_name" yaml:"db_name"`
}

var (
	cedarDBURLKey  = bsonutil.MustHaveTag(CedarConfig{}, "DBURL")
	cedarDBNameKey = bsonutil.MustHaveTag(CedarConfig{}, "DBName")
)

func (*CedarConfig) SectionId() string { return "cedar" }

func (c *CedarConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *CedarConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			cedarDBURLKey:  c.DBURL,
			cedarDBNameKey: c.DBName,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *CedarConfig) ValidateAndDefault() error { return nil }
