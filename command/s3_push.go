package command

import (
	"context"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// kim: TODO: figure out how to get a global evergreen S3 key/secret to this
// command (environment?)
// kim: TODO: figure out how to set up global private S3 bucket
// kim: TODO: figure out unique naming scheme for a given task/execution in S3.
// s3Push is a command to upload the task directory to s3.
type s3Push struct {
	// kim: TODO: how to set default values for these? These shouldn't be
	// configurable fields

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
	// catcher.NewWhen(c.DirPath == "", "directory path must be set")
	// catcher.NewWhen(c.AWSKey == "", "AWS key must be set")
	// catcher.NewWhen(c.AWSSecret == "", "AWS secret must be set")
	return catcher.Resolve()
}

func (c *s3Push) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	httpClient := util.GetHTTPClient()
	defer util.PutHTTPClient(httpClient)
	if err := c.createBucket(httpClient); err != nil {
		return errors.Wrap(err, "could not create S3 bucket")
	}

	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "could not check S3 bucket")
	}

	if err := c.bucket.Push(ctx, pail.SyncOptions{
		Local: conf.WorkDir,
		// kim: TODO: figure out remote once we have a bucket in S3.
		Remote:  "kim: TODO: remote_goes_here",
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "error pushing files to S3")
	}

	// TODO: project validation to ensure s3 push is not called multiple times.
	// logs, err := comm.S3Copy(ctx, client.TaskData{
	//     ID:     conf.Task.Id,
	//     Secret: conf.Task.Secret,
	// }, &apimodels.S3CopyRequest{
	//     AwsKey:              os.Getenv("S3_KEY"),
	//     AwsSecret:           os.Getenv("S3_SECRET"),
	//     S3SourceBucket:      "kim: TODO: remote_goes_here",
	//     S3SourcePath:        s3PushPrefixExecution(conf),
	//     S3DestinationBucket: "kim: TODO: remote_goes_here",
	//     S3DestinationPath:   s3PushPrefixLatest(conf),
	//     S3Permissions:       string(pail.S3PermissionsPrivate),
	// })
	// if logs != "" {
	//     logger.Task.Infof("s3 copy response: %s", logs)
	// }
	// if err != nil {
	//     return errors.Wrap(err, "failed to S3 copy pushed files to latest")
	// }

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
	// kim: TODO: get these into the agent environment
	s3Key := os.Getenv("S3_KEY")
	s3Secret := os.Getenv("S3_SECRET")
	// kim: TODO: use different bucket from this one
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Key == "" || s3Secret == "" || s3Bucket == "" {
		return errors.New("S3 credentials are missing")
	}
	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(s3Key, s3Secret, ""),
		Region:      endpoints.UsEast1RegionID,
		Name:        s3Bucket,
		// kim: TODO: check permissions
		Permissions: pail.S3PermissionsPrivate,
		// kim: TODO: figure out unique prefix using expansion values, which
		// will be taken from taskconfig
		// Default expansions: https://github.com/evergreen-ci/evergreen/wiki/Project-Files#default-expansions
		// Example output: https://evergreen.mongodb.com/task/jasper_ubuntu1604_test_cli_patch_7578fac81116661dc7005038fe0706aad28c9309_5e6b8b3a1e2d1717ba216501_20_03_13_13_31_39
		// Prefix: "${project_id}/${version_id}/${build_variant}/${task_name}/${execution}",
		Prefix: s3PushPrefixLatest(conf),
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

// s3PushPrefixLatest returns the path for the latest s3 push for this task.
func s3PushPrefixLatest(conf *model.TaskConfig) string {
	return path.Join(s3PushPrefixBase(conf), "latest")
}

// s3PushPrefixLatest returns the path for the s3 push for this task execution.
func s3PushPrefixExecution(conf *model.TaskConfig) string {
	return path.Join(s3PushPrefixBase(conf), strconv.Itoa(conf.Task.Execution))
}

// s3PushPrefixBase is a helper that returns the common s3 push path for a task.
func s3PushPrefixBase(conf *model.TaskConfig) string {
	return path.Join(conf.ProjectRef.Identifier, conf.Task.Version, conf.Task.BuildVariant, conf.Task.DisplayName)
}
