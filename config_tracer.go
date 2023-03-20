package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TracerConfig struct {
	Enabled           bool   `yaml:"enabled" bson:"enabled" json:"enabled"`
	CollectorEndpoint string `yaml:"collector_endpoint" bson:"collector_endpoint" json:"collector_endpoint"`
}

func (c *TracerConfig) SectionId() string { return "tracer" }

func (c *TracerConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)
	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = TracerConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *TracerConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			tracerEnabledKey:        c.Enabled,
			tracerCollectorEndpoint: c.CollectorEndpoint,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *TracerConfig) ValidateAndDefault() error {
	if c.Enabled && c.CollectorEndpoint == "" {
		return errors.New("tracer can't be enabled without a collector endpoint")
	}
	return nil
}
