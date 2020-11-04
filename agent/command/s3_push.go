package command

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/rest/client"
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
	return errors.Wrapf(c.s3Base.ParseParams(params), "error decoding %s params", c.Name())
}

func (c *s3Push) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "error applying expansions to parameters")
	}

	httpClient := utility.GetDefaultHTTPRetryableClient()
	defer utility.PutHTTPClient(httpClient)

	if err := c.createBucket(httpClient, conf); err != nil {
		return errors.Wrap(err, "could not set up S3 task bucket")
	}

	wd, err := conf.GetWorkingDirectory("")
	if err != nil {
		return errors.Wrap(err, "could not get working directory")
	}

	pushMsg := fmt.Sprintf("Pushing task directory files from %s into S3", wd)
	if c.ExcludeFilter != "" {
		pushMsg += ", excluding files matching filter " + c.ExcludeFilter
	}
	logger.Task().Infof(pushMsg)

	s3Path := conf.Task.S3Path(conf.Task.BuildVariant, conf.Task.DisplayName)
	if err := c.bucket.Push(ctx, pail.SyncOptions{
		Local:   wd,
		Remote:  s3Path,
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "error pushing task data to S3")
	}

	logger.Task().Infof("Successfully pushed task directory files")

	return nil
}
