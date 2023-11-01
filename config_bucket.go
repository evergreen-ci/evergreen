package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BucketType string

const (
	BucketTypeGridFS BucketType = "gridfs"
	BucketTypeLocal  BucketType = "local"
	BucketTypeS3     BucketType = "s3"
)

func (b BucketType) validate() error {
	switch b {
	case BucketTypeGridFS, BucketTypeLocal, BucketTypeS3:
		return nil
	default:
		return errors.Errorf("unrecognized bucket type '%s'", b)
	}
}

// BucketsConfig represents the admin config section for interally-owned
// Evergreen data bucket storage.
type BucketsConfig struct {
	LogBucket BucketConfig `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
}

var bucketsConfigLogBucketKey = bsonutil.MustHaveTag(BucketsConfig{}, "LogBucket")

// BucketConfig represents the admin config for an individual bucket.
type BucketConfig struct {
	Name   string     `bson:"name" json:"name" yaml:"name"`
	Type   BucketType `bson:"type" json:"type" yaml:"type"`
	DBName string     `bson:"db_name" json:"db_name" yaml:"db_name"`
}

func (c *BucketConfig) validate() error {
	if c.Type == "" {
		c.Type = BucketTypeS3
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(c.Type.validate())
	catcher.NewWhen(c.Type == BucketTypeGridFS && c.DBName == "", "must specify DB name for GridFS bucket")

	return catcher.Resolve()
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
