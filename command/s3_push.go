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
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// s3Push is a command to upload the task directory to s3.
type s3Push struct {
	// ExcludeFilter contains a regexp describing files that should be
	// excluded from the operation.
	ExcludeFilter string `mapstructure:"exclude_filter" plugin:"expand"`
	// BuildVariants contains all build variants this command should be run on.
	BuildVariants []string `mapstructure:"build_variants"`
	MaxRetries    uint     `mapstructure:"max_retries"`

	bucket pail.Bucket
	base
}

func s3PushFactory() Command { return &s3Push{} }

func (*s3Push) Name() string {
	return "s3.push"
}

const (
	defaultS3MaxRetries uint = 10
)

func (c *s3Push) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultS3MaxRetries
	}
	return nil
}

func (c *s3Push) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "error applying expansions to parameters")
	}

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	if !c.shouldRunOnBuildVariant(conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping s3.push for task directory for build variant '%s'", conf.BuildVariant.Name)
		return nil
	}

	if err := c.createBucket(httpClient, conf); err != nil {
		return errors.Wrap(err, "could not set up S3 task bucket")
	}

	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "could not find S3 task bucket")
	}

	wd, err := conf.GetWorkingDirectory("")
	if err != nil {
		return errors.Wrap(err, "could not get task working directory")
	}
	putMsg := "Pushing task directory files into S3"
	if c.ExcludeFilter != "" {
		putMsg += ", excluding files matching filter " + c.ExcludeFilter
	}
	logger.Task().Infof(putMsg)
	if err := c.bucket.Push(ctx, pail.SyncOptions{
		Local:   wd,
		Remote:  conf.S3Path(),
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
