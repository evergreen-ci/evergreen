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
	BucketTypeGridFS = "gridfs"
	BucketTypeLocal  = "local"
	BucketTypeS3     = "s3"
)

// BucketsConfig represents the admin config section for bucket storage.
type BucketsConfig struct {
	LogBucket BucketConfig `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
}

var bucketsConfigLogBucketKey = bsonutil.MustHaveTag(BucketsConfig{}, "LogBucket")

// BucketConfig represents the admin config for an individual bucket.
type BucketConfig struct {
	Name   string `bson:"name" json:"name" yaml:"name"`
	Type   string `bson:"type" json:"type" yaml:"type"`
	DBName string `bson:"db_name" json:"db_name" yaml:"db_name"`
}

func (c *BucketConfig) validate() error {
	if c.Type == "" {
		c.Type = BucketTypeS3
	}
	if c.Type == BucketTypeGridFS && c.DBName == "" {
		return errors.New("must specify DB name for GridFS bucket")
	}

	return nil
}

func (*BucketsConfig) SectionId() string { return "buckets" }

func (c *BucketsConfig) Get(ctx context.Context) error {
	coll := GetEnvironment().DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = BucketsConfig{}
			return nil
		}
		return errors.Wrapf(err, "retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "decoding result")
	}

	return nil
}

func (c *BucketsConfig) Set(ctx context.Context) error {
	coll := GetEnvironment().DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			bucketsConfigLogBucketKey: c.LogBucket,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating section %s", c.SectionId())
}

func (c *BucketsConfig) ValidateAndDefault() error {
	return c.LogBucket.validate()
}
