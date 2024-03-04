package main

import (
	"context"
	"flag"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

// Upload a directory/file to S3.
func main() {
	var bucketName, local, remote, exclude string
	flag.StringVar(&bucketName, "bucket", "", "the S3 bucket name")
	flag.StringVar(&local, "local", "", "the local path")
	flag.StringVar(&remote, "remote", "", "the remote path")
	flag.StringVar(&exclude, "exclude", "", "file pattern to exclude from  upload")
	flag.Parse()
	if bucketName == "" {
		grip.EmergencyFatal("bucket name is missing")
	}
	if local == "" {
		grip.EmergencyFatal("local path is missing")
	}
	if remote == "" {
		grip.EmergencyFatal("remote path is missing")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := utility.GetDefaultHTTPRetryableClient()
	defer utility.PutHTTPClient(client)

	opts := pail.S3Options{
		Region:      endpoints.UsEast1RegionID,
		Name:        bucketName,
		MaxRetries:  utility.ToIntPtr(10),
		Permissions: pail.S3PermissionsPublicRead,
		Verbose:     true,
	}
	bucket, err := pail.NewS3Bucket(opts)
	if err != nil {
		grip.EmergencyFatal("could not initialize bucket")
	}

	if err := grip.SetLevel(send.LevelInfo{Threshold: level.Debug, Default: level.Debug}); err != nil {
		grip.EmergencyFatal(errors.Wrap(err, "setting grip logging level"))
	}

	if err := bucket.Push(ctx, pail.SyncOptions{
		Local:   local,
		Remote:  remote,
		Exclude: exclude,
	}); err != nil {
		grip.EmergencyFatal(errors.Wrapf(err, "pushing contents from local path %s to remote path %s in bucket %s", local, remote, bucketName))
	}
}
