package command

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
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
	return errors.Wrapf(mapstructure.Decode(params, c), "decoding mapstructure params")
}

func (c *s3Base) expandParams(conf *internal.TaskConfig) error {
	return errors.Wrap(util.ExpandValues(c, &conf.Expansions), "applying expansions")
}

func (c *s3Base) createBucket(client *http.Client, conf *internal.TaskConfig) error {
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
		MaxRetries:  utility.ToIntPtr(int(c.MaxRetries)),
		Permissions: pail.S3PermissionsPrivate,
	}

	bucket, err := pail.NewS3ArchiveBucketWithHTTPClient(client, opts)
	if err != nil {
		return errors.Wrap(err, "initializing bucket")
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
		return errors.Wrap(err, "parsing common S3 params")
	}
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}
	if c.Task == "" {
		return errors.New("task must not be empty")
	}
	return nil
}

func (c *s3Pull) expandParams(conf *internal.TaskConfig) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(c.s3Base.expandParams(conf))
	catcher.Add(util.ExpandValues(c, &conf.Expansions))
	return catcher.Resolve()
}

func (c *s3Pull) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "applying expansions")
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
		return errors.Wrap(err, "creating S3 task bucket")
	}

	remotePath := conf.Task.S3Path(c.FromBuildVariant, c.Task)

	// Verify that the contents exist before pulling, otherwise pull may no-op.

	logger.Execution().WarningWhen(filepath.IsAbs(c.WorkingDir) && !strings.HasPrefix(c.WorkingDir, conf.WorkDir),
		fmt.Sprintf("The working directory ('%s') is an absolute path, which isn't supported except when prefixed by '%s'.",
			c.WorkingDir, conf.WorkDir))

	if err := createEnclosingDirectoryIfNeeded(c.WorkingDir); err != nil {
		return errors.Wrapf(err, "creating parent directories for working directory '%s'", c.WorkingDir)
	}
	wd, err := getWorkingDirectoryLegacy(conf, c.WorkingDir)
	if err != nil {
		return errors.Wrapf(err, "getting working directory")
	}

	pullMsg := fmt.Sprintf("Pulling task directory files from S3 from task '%s' on build variant '%s'", c.Task, c.FromBuildVariant)
	if c.ExcludeFilter != "" {
		pullMsg = fmt.Sprintf("%s, excluding files matching filter '%s'", pullMsg, c.ExcludeFilter)
	}
	pullMsg += "."
	logger.Task().Infof(pullMsg)
	if err = c.bucket.Pull(ctx, pail.SyncOptions{
		Local:   wd,
		Remote:  remotePath,
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "pulling task data from S3")
	}

	logger.Task().Info("Successfully pulled task directory files.")

	return nil
}
