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
	BucketTypeGridFS    BucketType = "gridfs"
	BucketTypeLocal     BucketType = "local"
	BucketTypeS3        BucketType = "s3"
	DefaultS3Region                = "us-east-1"
	DefaultS3MaxRetries            = 10
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
	// LogBucket is the default bucket information for logs.
	LogBucket BucketConfig `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
	// LogBucketLongRetention is the bucket information for logs with extended retention.
	LogBucketLongRetention BucketConfig `bson:"log_bucket_long_retention" json:"log_bucket_long_retention" yaml:"log_bucket_long_retention"`
	// LongRetentionProjects is the list of project IDs that require long retention.
	LongRetentionProjects []string `bson:"long_retention_projects" json:"long_retention_projects" yaml:"long_retention_projects"`
	// TestResultsBucket is the bucket information for test results.
	TestResultsBucket BucketConfig `bson:"test_results_bucket" json:"test_results_bucket" yaml:"test_results_bucket"`
	// Credentials for accessing the LogBucket.
	Credentials S3Credentials `bson:"credentials" json:"credentials" yaml:"credentials"`
}

var (
	bucketsConfigLogBucketKey              = bsonutil.MustHaveTag(BucketsConfig{}, "LogBucket")
	bucketsConfigLogBucketLongRetentionKey = bsonutil.MustHaveTag(BucketsConfig{}, "LogBucketLongRetention")
	bucketsConfigLongRetentionProjectsKey  = bsonutil.MustHaveTag(BucketsConfig{}, "LongRetentionProjects")
	bucketsConfigTestResultsBucketKey      = bsonutil.MustHaveTag(BucketsConfig{}, "TestResultsBucket")
	bucketsConfigCredentialsKey            = bsonutil.MustHaveTag(BucketsConfig{}, "Credentials")
)

// BucketConfig represents the admin config for an individual bucket.
type BucketConfig struct {
	Name              string     `bson:"name" json:"name" yaml:"name"`
	Type              BucketType `bson:"type" json:"type" yaml:"type"`
	DBName            string     `bson:"db_name" json:"db_name" yaml:"db_name"`
	TestResultsPrefix string     `bson:"test_results_prefix" json:"test_results_prefix" yaml:"test_results_prefix"`
	RoleARN           string     `bson:"role_arn" json:"role_arn" yaml:"role_arn"`
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
			bucketsConfigLogBucketKey:              c.LogBucket,
			bucketsConfigLogBucketLongRetentionKey: c.LogBucketLongRetention,
			bucketsConfigLongRetentionProjectsKey:  c.LongRetentionProjects,
			bucketsConfigTestResultsBucketKey:      c.TestResultsBucket,
			bucketsConfigCredentialsKey:            c.Credentials,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *BucketsConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(c.LogBucket.validate())
	catcher.Add(c.LogBucketLongRetention.validate())
	return catcher.Resolve()
}
