package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// DiagnosticsConfig holds settings for Evergreen app server diagnostics.
type DiagnosticsConfig struct {
	S3BucketName string `yaml:"s3_bucket_name" bson:"s3_bucket_name" json:"s3_bucket_name"`
	S3Prefix     string `yaml:"s3_prefix" bson:"s3_prefix" json:"s3_prefix"`
}

func (c *DiagnosticsConfig) SectionId() string { return "diagnostics" }

func (c *DiagnosticsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *DiagnosticsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			diagnosticsS3BucketNameKey: c.S3BucketName,
			diagnosticsS3PrefixKey:     c.S3Prefix,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *DiagnosticsConfig) ValidateAndDefault() error { return nil }
