package taskoutput

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func newBucket(bucketName, bucketType string) (pail.Bucket, error) {
	env := evergreen.GetEnvironment()

	var (
		b   pail.Bucket
		err error
	)
	switch bucketType {
	case evergreen.BucketTypeS3:
		b, err = pail.NewS3Bucket(pail.S3Options{
			Name:        bucketName,
			Region:      evergreen.DefaultEC2Region,
			Permissions: pail.S3PermissionsPrivate,
			MaxRetries:  utility.ToIntPtr(10),
			Compress:    true,
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
	case evergreen.BucketTypeGridFS:
		b, err = pail.NewGridFSBucketWithClient(context.Background(), env.Client(), pail.GridFSOptions{
			Name:     bucketName,
			Database: env.DB().Name(),
		})
		if err != nil {
			return nil, errors.WithStack(err)
		}
	case evergreen.BucketTypeLocal:
		b, err = pail.NewLocalBucket(pail.LocalOptions{Path: bucketName})
		if err != nil {
			return nil, errors.WithStack(err)
		}
	default:
		return nil, errors.Errorf("unrecognized bucket type '%s'", bucketType)
	}

	return b, nil
}
