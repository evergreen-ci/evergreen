package evergreen

import (
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	BucketTypeLocal = "local"
	BucketTypeS3    = "s3"
)

// BucketConfig represents the admin config section for bucket storage.
type BucketConfig struct {
	LogBucket     string `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
	LogBucketType string `bson:"log_bucket_type" json:"log_bucket_type" yaml:"log_bucket_type"`
}

var (
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
			bucketConfigLogBucketKey:     c.LogBucket,
			bucketConfigLogBucketTypeKey: c.LogBucketType,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating section %s", c.SectionId())
}

func (c *BucketConfig) ValidateAndDefault() error {
	if c.LogBucketType == "" {
		c.LogBucketType = BucketTypeS3
	}

	return nil
}
