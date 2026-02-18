package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// S3LifecycleClient fetches S3 lifecycle configurations using pail.
type S3LifecycleClient interface {
	// GetBucketLifecycleConfiguration returns lifecycle rules for the specified S3 bucket.
	GetBucketLifecycleConfiguration(ctx context.Context, bucket, region string, roleARN *string, externalID *string) ([]pail.LifecycleRule, error)
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
func (c *s3LifecycleClientImpl) GetBucketLifecycleConfiguration(ctx context.Context, bucketName, region string, roleARN *string, externalID *string) ([]pail.LifecycleRule, error) {
	if bucketName == "" {
		return nil, errors.New("bucket name cannot be empty")
	}
	if region == "" {
		return nil, errors.New("region cannot be empty")
	}

	opts := pail.S3Options{
		Name:       bucketName,
		Region:     region,
		MaxRetries: utility.ToIntPtr(evergreen.DefaultS3MaxRetries),
	}
	if roleARN != nil && *roleARN != "" {
		opts.AssumeRoleARN = *roleARN
		if externalID != nil && *externalID != "" {
			opts.AssumeRoleOptions = []func(*stscreds.AssumeRoleOptions){
				func(aro *stscreds.AssumeRoleOptions) {
					aro.ExternalID = externalID
				},
			}
		}
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
