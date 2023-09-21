package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DataPipesConfig represents the admin config section for the Data-Pipes
// results service. See `https://github.com/10gen/data-pipes` for more
// information.
type DataPipesConfig struct {
	Host         string `bson:"host" json:"host" yaml:"host"`
	Region       string `bson:"region" json:"region" yaml:"region"`
	AWSAccessKey string `bson:"aws_access_key" json:"aws_access_key" yaml:"aws_access_key"`
	AWSSecretKey string `bson:"aws_secret_key" json:"aws_secret_key" yaml:"aws_secret_key"`
	AWSToken     string `bson:"aws_token" json:"aws_token" yaml:"aws_token"`
}

var (
	dataPipesConfigHostKey         = bsonutil.MustHaveTag(DataPipesConfig{}, "Host")
	dataPipesConfigRegionKey       = bsonutil.MustHaveTag(DataPipesConfig{}, "Region")
	dataPipesConfigAWSAccessKeyKey = bsonutil.MustHaveTag(DataPipesConfig{}, "AWSAccessKey")
	dataPipesConfigAWSSecretKeyKey = bsonutil.MustHaveTag(DataPipesConfig{}, "AWSSecretKey")
	dataPipesConfigAWSTokenKey     = bsonutil.MustHaveTag(DataPipesConfig{}, "AWSToken")
)

func (*DataPipesConfig) SectionId() string { return "data_pipes" }

func (c *DataPipesConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = DataPipesConfig{}
			return nil
		}
		return errors.Wrapf(err, "retrieving section %s", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrap(err, "decoding result")
	}

	return nil
}

func (c *DataPipesConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			dataPipesConfigHostKey:         c.Host,
			dataPipesConfigRegionKey:       c.Region,
			dataPipesConfigAWSAccessKeyKey: c.AWSAccessKey,
			dataPipesConfigAWSSecretKeyKey: c.AWSSecretKey,
			dataPipesConfigAWSTokenKey:     c.AWSToken,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating section %s", c.SectionId())
}

func (c *DataPipesConfig) ValidateAndDefault() error { return nil }
