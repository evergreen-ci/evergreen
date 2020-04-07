package command

import (
	"context"
	"runtime"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
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
		Remote:  conf.Task.S3Path(conf.Task.BuildVariant, conf.Task.DisplayName),
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "error pushing task data to S3")
	}

	return nil
}
