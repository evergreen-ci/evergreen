package cloud

import (
	"context"

	"github.com/evergreen-ci/pail"
	"github.com/pkg/errors"
)

// S3LifecycleClient fetches S3 lifecycle configurations using pail.
type S3LifecycleClient interface {
	// GetBucketLifecycleConfiguration returns lifecycle rules for the specified S3 bucket.
	GetBucketLifecycleConfiguration(ctx context.Context, bucket, region string, roleARN *string) ([]pail.LifecycleRule, error)
}

// s3LifecycleBucket extends pail.Bucket with GetLifecycleConfiguration.
// The concrete s3Bucket type implements this but pail.Bucket interface doesn't include it.
type s3LifecycleBucket interface {
	pail.Bucket
	GetLifecycleConfiguration(ctx context.Context) ([]pail.LifecycleRule, error)
}

type s3LifecycleClientImpl struct{}

// NewS3LifecycleClient creates a new S3LifecycleClient for fetching S3 bucket lifecycle configurations.
func NewS3LifecycleClient() S3LifecycleClient {
	return &s3LifecycleClientImpl{}
}

// GetBucketLifecycleConfiguration returns lifecycle rules for the specified S3 bucket.
func (c *s3LifecycleClientImpl) GetBucketLifecycleConfiguration(ctx context.Context, bucketName, region string, roleARN *string) ([]pail.LifecycleRule, error) {
	if bucketName == "" {
		return nil, errors.New("bucket name cannot be empty")
	}
	if region == "" {
		return nil, errors.New("region cannot be empty")
	}

	opts := pail.S3Options{
		Name:   bucketName,
		Region: region,
	}
	if roleARN != nil && *roleARN != "" {
		opts.AssumeRoleARN = *roleARN
	}

	bucket, err := pail.NewS3Bucket(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "creating pail S3 bucket")
	}

	s3Bucket, ok := bucket.(s3LifecycleBucket)
	if !ok {
		return nil, errors.New("bucket does not support lifecycle configuration")
	}

	rules, err := s3Bucket.GetLifecycleConfiguration(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting lifecycle configuration from pail")
	}

	return rules, nil
}
