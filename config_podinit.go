package evergreen

import (
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var podInitConfigKey = bsonutil.MustHaveTag(Settings{}, "PodInit")

// PodInitConfig holds logging settings for the pod init process.
type PodInitConfig struct {
	S3BaseURL string `bson:"s3_base_url" json:"s3_base_url" yaml:"s3_base_url"`
}

func (c *PodInitConfig) SectionId() string { return "pod_init" }

func (c *PodInitConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = PodInitConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *PodInitConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			podInitS3BaseURLKey: c.S3BaseURL,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *PodInitConfig) ValidateAndDefault() error {
	return nil
}
