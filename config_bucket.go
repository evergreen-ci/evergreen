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

// ProjectToPrefixMapping relates a project to a bucket path prefix.
type ProjectToPrefixMapping struct {
	// ProjectID is the project's ID.
	ProjectID string `yaml:"project_id" bson:"project_id" json:"project_id"`
	// Prefix is the bucket path prefix that the project should have access to.
	Prefix string `yaml:"prefix" bson:"prefix" json:"prefix"`
}

// BucketsConfig represents the admin config section for interally-owned
// Evergreen data bucket storage.
type BucketsConfig struct {
	// LogBucket is the bucket information for logs.
	LogBucket BucketConfig `bson:"log_bucket" json:"log_bucket" yaml:"log_bucket"`
	// Credentials for accessing the LogBucket.
	Credentials S3Credentials `bson:"credentials" json:"credentials" yaml:"credentials"`

	// InternalBuckets are the buckets that Evergreen's app servers have access to
	// via their IRSA role.
	InternalBuckets []string `yaml:"internal_buckets" bson:"internal_buckets" json:"internal_buckets"`

	// ProjectToPrefixMappings is a list of project to prefix mappings.
	// This is used to connect cross-project access to the same prefix.
	// E.g. if project A should have access to project B's prefix, then
	// project A's ID and project B's prefix should be in this list.
	ProjectToPrefixMappings []ProjectToPrefixMapping `yaml:"project_to_prefix_mappings" bson:"project_to_prefix_mappings" json:"project_to_prefix_mappings"`
}

var (
	bucketsConfigLogBucketKey       = bsonutil.MustHaveTag(BucketsConfig{}, "LogBucket")
	bucketsConfigCredentialsKey     = bsonutil.MustHaveTag(BucketsConfig{}, "Credentials")
	bucketsConfigInternalBucketsKey = bsonutil.MustHaveTag(BucketsConfig{}, "InternalBuckets")
	projectToPrefixMappingsKey      = bsonutil.MustHaveTag(BucketsConfig{}, "ProjectToPrefixMappings")
)

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
			bucketsConfigLogBucketKey:       c.LogBucket,
			bucketsConfigCredentialsKey:     c.Credentials,
			bucketsConfigInternalBucketsKey: c.InternalBuckets,
			projectToPrefixMappingsKey:      c.ProjectToPrefixMappings,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *BucketsConfig) ValidateAndDefault() error {
	return c.LogBucket.validate()
}
