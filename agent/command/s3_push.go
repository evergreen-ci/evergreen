package command

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
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
	return evergreen.S3PushCommandName
}

func (c *s3Push) ParseParams(params map[string]interface{}) error {
	return errors.Wrapf(c.s3Base.ParseParams(params), "parsing common S3 parameters")
}

func (c *s3Push) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	httpClient := utility.GetDefaultHTTPRetryableClient()
	defer utility.PutHTTPClient(httpClient)

	if err := c.createBucket(httpClient, conf); err != nil {
		return errors.Wrap(err, "creating S3 task bucket")
	}

	wd, err := getWorkingDirectoryLegacy(conf, "")
	if err != nil {
		return errors.Wrap(err, "getting working directory")
	}

	pushMsg := fmt.Sprintf("Pushing task directory files from directory '%s' into S3", wd)
	if c.ExcludeFilter != "" {
		pushMsg = fmt.Sprintf("%s, excluding files matching filter '%s'", pushMsg, c.ExcludeFilter)
	}
	pushMsg += "."
	logger.Task().Infof(pushMsg)

	s3Path := conf.Task.S3Path(conf.Task.BuildVariant, conf.Task.DisplayName)
	if err := c.bucket.Push(ctx, pail.SyncOptions{
		Local:   wd,
		Remote:  s3Path,
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "pushing task data to S3")
	}

	logger.Task().Info("Successfully pushed task directory files.")

	return nil
}
