package evergreen

import (
	"context"

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
	LogBucket Bucket `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
}

var bucketConfigLogBucketKey = bsonutil.MustHaveTag(BucketConfig{}, "LogBucket")

// Bucket represents the admin config for an individual bucket.
type Bucket struct {
	Name string `bson:"name" json:"name" yaml:"name"`
	Type string `bson:"type" json:"type" yaml:"type"`
}

func (*BucketConfig) SectionId() string { return "buckets" }

func (c *BucketConfig) Get(ctx context.Context) error {
	coll := GetEnvironment().DB().Collection(ConfigCollection)

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

func (c *BucketConfig) Set(ctx context.Context) error {
	coll := GetEnvironment().DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			bucketConfigLogBucketKey: c.LogBucket,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating section %s", c.SectionId())
}

func (c *BucketConfig) ValidateAndDefault() error {
	if c.LogBucket.Type == "" {
		c.LogBucket.Type = BucketTypeS3
	}

	return nil
}
