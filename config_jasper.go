package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type JasperConfig struct {
	BinaryName string `yaml:"binary_name" bson:"binary_name" json:"binary_name"`
	Port       int    `yaml:"port" bson:"port" json:"port"`
	URL        string `yaml:"url" bson:"url" json:"url"`
	Version    string `yaml:"version" bson:"version" json:"version"`
}

func (c *JasperConfig) SectionId() string { return "jasper" }

func (c *JasperConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)
	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = JasperConfig{}
			return nil
		}

		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *JasperConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			jasperBinaryNameKey: c.BinaryName,
			jasperPortKey:       c.Port,
			jasperURLKey:        c.URL,
			jasperVersionKey:    c.Version,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *JasperConfig) ValidateAndDefault() error {
	if c.Port <= 0 {
		return errors.Errorf("Jasper port must be a positive integer")
	}
	return nil
}
