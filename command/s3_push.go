package command

import (
	"context"
	"net/http"
	"runtime"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// s3Push is a command to upload the task directory to s3.
type s3Push struct {
	s3Base
	base
}

func s3PushFactory() Command { return &s3Push{} }

func (*s3Push) Name() string {
	return "s3.push"
}

func (c *s3Push) ParseParams(params map[string]interface{}) error {
	if err := c.s3Base.ParseParams(params); err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}
	return nil
}

func (c *s3Push) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "error applying expansions to parameters")
	}

	if !c.shouldRunOnBuildVariant(conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping s3.push for build variant '%s'", conf.BuildVariant.Name)
		return nil
	}

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	if err := c.createBucket(httpClient, conf, pail.ParallelBucketOptions{
		Workers:      runtime.NumCPU(),
		DeleteOnSync: true,
	}); err != nil {
		return errors.Wrap(err, "could not set up S3 task bucket")
	}
	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "could not find S3 task bucket")
	}

	wd, err := conf.GetWorkingDirectory("")
	if err != nil {
		return errors.Wrap(err, "could not get task working directory")
	}
	pushMsg := "Pushing task directory files into S3"
	if c.ExcludeFilter != "" {
		pushMsg += ", excluding files matching filter " + c.ExcludeFilter
	}
	logger.Task().Infof(pushMsg)
	if err := c.bucket.Push(ctx, pail.SyncOptions{
		Local:   wd,
		Remote:  conf.Task.S3Path(conf.Task.DisplayName),
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "error pushing task data to S3")
	}

	return nil
}

func (c *s3Push) expandParams(conf *model.TaskConfig) error {
	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (c *s3Push) shouldRunOnBuildVariant(bv string) bool {
	if len(c.BuildVariants) == 0 {
		return true
	}
	return utility.StringSliceContains(c.BuildVariants, bv)
}

func (c *s3Push) createBucket(client *http.Client, conf *model.TaskConfig) error {
	if c.bucket != nil {
		return nil
	}

	if err := conf.S3Data.Validate(); err != nil {
		return errors.Wrap(err, "invalid S3 task credentials")
	}

	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(conf.S3Data.Key, conf.S3Data.Secret, ""),
		Region:      endpoints.UsEast1RegionID,
		Name:        conf.S3Data.Bucket,
		MaxRetries:  int(c.MaxRetries),
		Permissions: pail.S3PermissionsPrivate,
	}
	bucket, err := pail.NewS3MultiPartBucketWithHTTPClient(client, opts)
	if err != nil {
		return errors.Wrap(err, "could not create bucket")
	}
	bucket = pail.NewParallelSyncBucket(pail.ParallelBucketOptions{
		Workers:      runtime.NumCPU(),
		DeleteOnSync: true,
	}, bucket)
	c.bucket = bucket

	return nil
}
