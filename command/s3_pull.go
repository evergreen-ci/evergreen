package command

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type s3Base struct {
	// ExcludeFilter contains a regexp describing files that should be
	// excluded from the operation.
	ExcludeFilter string `mapstructure:"exclude" plugin:"expand"`
	MaxRetries    uint   `mapstructure:"max_retries"`

	bucket pail.SyncBucket
}

func (c *s3Base) ParseParams(params map[string]interface{}) error {
	return errors.Wrapf(mapstructure.Decode(params, c), "error decoding S3 parameters")
}

func (c *s3Base) expandParams(conf *model.TaskConfig) error {
	return errors.WithStack(util.ExpandValues(c, conf.Expansions))
}

func (c *s3Base) createBucket(client *http.Client, conf *model.TaskConfig) error {
	if c.bucket != nil {
		return nil
	}

	if err := conf.TaskSync.Validate(); err != nil {
		return errors.Wrap(err, "invalid credentials for task sync")
	}

	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(conf.TaskSync.Key, conf.TaskSync.Secret, ""),
		Region:      endpoints.UsEast1RegionID,
		Name:        conf.TaskSync.Bucket,
		MaxRetries:  int(c.MaxRetries),
		Permissions: pail.S3PermissionsPrivate,
	}
	bucket, err := pail.NewS3ArchiveBucketWithHTTPClient(client, opts)
	if err != nil {
		return errors.Wrap(err, "could not create bucket")
	}
	c.bucket = bucket

	return nil
}

// s3Pull is a command to download the task directory from S3.
type s3Pull struct {
	s3Base
	base

	FromBuildVariant string `mapstructure:"from_build_variant"`
	Task             string `mapstructure:"task"`
	WorkingDir       string `mapstructure:"working_directory" plugin:"expand"`
	DeleteOnSync     bool   `mapstructure:"delete_on_sync"`
}

func s3PullFactory() Command { return &s3Pull{} }

func (*s3Pull) Name() string {
	return "s3.pull"
}

func (c *s3Pull) ParseParams(params map[string]interface{}) error {
	if err := c.s3Base.ParseParams(params); err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}
	if c.Task == "" {
		return errors.New("task must not be empty")
	}
	return nil
}

func (c *s3Pull) expandParams(conf *model.TaskConfig) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(c.s3Base.expandParams(conf))
	catcher.Add(util.ExpandValues(c, conf.Expansions))
	return catcher.Resolve()
}

func (c *s3Pull) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "error applying expansions to parameters")
	}
	if c.FromBuildVariant == "" {
		c.FromBuildVariant = conf.Task.BuildVariant
	}
	if c.WorkingDir == "" {
		c.WorkingDir = conf.WorkDir
	}

	httpClient := utility.GetDefaultHTTPRetryableClient()
	// Do not time out a download since it could be an expensive operation
	// depending on the download speed and the size of the pull.
	httpClient.Timeout = 0
	defer utility.PutHTTPClient(httpClient)

	if err := c.createBucket(httpClient, conf); err != nil {
		return errors.Wrap(err, "could not set up S3 task bucket")
	}

	remotePath := conf.Task.S3Path(c.FromBuildVariant, c.Task)

	// Verify that the contents exist before pulling, otherwise pull may no-op.

	logger.Execution().WarningWhen(filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		fmt.Sprintf("the working directory ('%s') is an absolute path, which isn't supported except when prefixed by '%s'",
			c.WorkingDir, conf.WorkDir))

	if err := createEnclosingDirectoryIfNeeded(c.WorkingDir); err != nil {
		return errors.Wrap(err, "problem making working directory")
	}
	wd, err := conf.GetWorkingDirectory(c.WorkingDir)
	if err != nil {
		return errors.Wrap(err, "could not get working directory")
	}

	pullMsg := fmt.Sprintf("Pulling task directory files from S3 from task '%s' on build variant '%s'", c.Task, c.FromBuildVariant)
	if c.ExcludeFilter != "" {
		pullMsg += ", excluding files matching filter " + c.ExcludeFilter
	}
	logger.Task().Infof(pullMsg)
	if err = c.bucket.Pull(ctx, pail.SyncOptions{
		Local:   wd,
		Remote:  remotePath,
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "error pulling task data from S3")
	}

	return nil
}
