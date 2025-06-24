package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type RuntimeEnvironmentsConfig struct {
	BaseURL string `yaml:"base_url" bson:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" bson:"api_key" json:"api_key" secret:"true"`
}

var (
	runtimeEnvironmentsBaseURLKey = bsonutil.MustHaveTag(RuntimeEnvironmentsConfig{}, "BaseURL")
	runtimeEnvironmentsAPIKey     = bsonutil.MustHaveTag(RuntimeEnvironmentsConfig{}, "APIKey")
)

func (*RuntimeEnvironmentsConfig) SectionId() string { return "runtime_environments" }

func (c *RuntimeEnvironmentsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *RuntimeEnvironmentsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			runtimeEnvironmentsBaseURLKey: c.BaseURL,
			runtimeEnvironmentsAPIKey:     c.APIKey,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *RuntimeEnvironmentsConfig) ValidateAndDefault() error { return nil }
