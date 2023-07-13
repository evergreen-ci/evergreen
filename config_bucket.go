package evergreen

import (
	"fmt"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BucketConfig represents the admin config section for bucket storage.
type BucketConfig struct {
	AWSKey    string `bson:"aws_key,omitempty" json:"aws_key,omitempty" yaml:"aws_key,omitempty"`
	AWSSecret string `bson:"aws_secret,omitempty" json:"aws_secret,omitempty" yaml:"aws_secret,omitempty"`

	LogBucket     string `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
	LogBucketType string `bson:"log_bucket_type" json:"log_bucket_type" yaml:"log_bucket_type"`
}

var (
	bucketConfigAWSKeyKey        = bsonutil.MustHaveTag(BucketConfig{}, "AWSKey")
	bucketConfigAWSSecretKey     = bsonutil.MustHaveTag(BucketConfig{}, "AWSSecret")
	bucketConfigLogBucketKey     = bsonutil.MustHaveTag(BucketConfig{}, "LogBucket")
	bucketConfigLogBucketTypeKey = bsonutil.MustHaveTag(BucketConfig{}, "LogBucketType")
)

func (*BucketConfig) SectionId() string { return "bucket" }

func (c *BucketConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			fmt.Println("HERE")
			*c = BucketConfig{}
			return nil
		}
		return errors.Wrapf(err, "retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "decoding result")
	}

	return nil
}

func (c *BucketConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			bucketConfigAWSKeyKey:        c.AWSKey,
			bucketConfigAWSSecretKey:     c.AWSSecret,
			bucketConfigLogBucketKey:     c.LogBucket,
			bucketConfigLogBucketTypeKey: c.LogBucketType,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating section %s", c.SectionId())
}

func (c *BucketConfig) ValidateAndDefault() error { return nil }
