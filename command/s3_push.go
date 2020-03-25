package command

import (
	"context"
	"net/http"
	"path"
	"runtime"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// s3Push is a command to upload the task directory to s3.
type s3Push struct {
	// ExcludeFilter contains a regexp describing files that should be
	// excluded from the operation.
	ExcludeFilter string `mapstructure:"exclude_filter" plugin:"expand"`
	// BuildVariants contains all build variants this command should be run on.
	BuildVariants []string `mapstructure:"build_variants"`

	bucket pail.Bucket
	base
}

func s3PushFactory() Command { return &s3Push{} }

func (*s3Push) Name() string {
	return "s3.push"
}

func (c *s3Push) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}
	if err := c.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating %s params", c.Name())
	}
	return nil
}

func (c *s3Push) validateParams() error {
	catcher := grip.NewBasicCatcher()
	// kim: TODO: validate buildvariants?
	return catcher.Resolve()
}

func (c *s3Push) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	httpClient := util.GetHTTPClient()
	defer util.PutHTTPClient(httpClient)

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

	putMsg := "Pushing task directory files into S3"
	if c.ExcludeFilter != "" {
		putMsg += ", excluding files matching filter " + c.ExcludeFilter
	}
	logger.Task().Infof(putMsg)
	if err := c.bucket.Push(ctx, pail.SyncOptions{
		Local:   conf.WorkDir,
		Remote:  s3TaskRemotePath(conf),
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
	return util.StringSliceContains(c.BuildVariants, bv)
}

func (c *s3Push) createBucket(client *http.Client, conf *model.TaskConfig) error {
	if c.bucket != nil {
		return nil
	}
	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(conf.S3Data.Key, conf.S3Data.Secret, ""),
		Region:      endpoints.UsEast1RegionID,
		Name:        conf.S3Data.Bucket,
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

// s3TaskRemotePath returns the path for the latest s3 push for this task.
func s3TaskRemotePath(conf *model.TaskConfig) string {
	return path.Join(s3TaskRemotePathBase(conf), "latest")
}

// s3TaskPrefixBase is a helper that returns the base s3 path in S3, which is a
// path path unique to a task.
func s3TaskRemotePathBase(conf *model.TaskConfig) string {
	return path.Join(conf.ProjectRef.Identifier, conf.Task.Version, conf.Task.BuildVariant, conf.Task.DisplayName)
}
