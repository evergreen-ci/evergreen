package taskoutput

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func newBucket(ctx context.Context, env evergreen.Environment, bucketName, bucketType string) (pail.Bucket, error) {
	switch bucketType {
	case evergreen.BucketTypeS3:
		return pail.NewS3Bucket(pail.S3Options{
			Name:        bucketName,
			Region:      evergreen.DefaultEC2Region,
			Permissions: pail.S3PermissionsPrivate,
			MaxRetries:  utility.ToIntPtr(10),
			Compress:    true,
		})
	case evergreen.BucketTypeGridFS:
		return pail.NewGridFSBucketWithClient(ctx, env.Client(), pail.GridFSOptions{
			Name:     bucketName,
			Database: env.DB().Name(),
		})
	case evergreen.BucketTypeLocal:
		return pail.NewLocalBucket(pail.LocalOptions{Path: bucketName})
	default:
		return nil, errors.Errorf("unrecognized bucket type '%s'", bucketType)
	}
}
