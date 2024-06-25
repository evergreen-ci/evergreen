package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type RuntimeEnvironmentsConfig struct {
	BaseURL string `yaml:"base_url" bson:"base_url" json:"base_url"`
	APIKey  string `yaml:"api_key" bson:"api_key" json:"api_key"`
}

func (*RuntimeEnvironmentsConfig) SectionId() string { return "runtimeenvironments" }

func (c *RuntimeEnvironmentsConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = RuntimeEnvironmentsConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *RuntimeEnvironmentsConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"base_url": c.BaseURL,
			"api_key":  c.APIKey,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *RuntimeEnvironmentsConfig) ValidateAndDefault() error { return nil }
