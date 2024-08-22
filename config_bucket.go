package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	return getConfigSection(ctx, c)
}

func (c *BucketsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			bucketsConfigLogBucketKey: c.LogBucket,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *BucketsConfig) ValidateAndDefault() error {
	return c.LogBucket.validate()
}
