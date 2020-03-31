package command

import (
	"context"
	"runtime"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
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

	httpClient := util.GetHTTPClient()
	defer util.PutHTTPClient(httpClient)

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
		Remote:  conf.S3Path(),
		Exclude: c.ExcludeFilter,
	}); err != nil {
		return errors.Wrap(err, "error pushing task data to S3")
	}

	return nil
}
