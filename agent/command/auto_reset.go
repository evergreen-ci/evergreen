package command

import (
	"context"
	"github.com/pkg/errors"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
)

type autoReset struct{ base }

func autoResetFactory() Command                                      { return &autoReset{} }
func (c *autoReset) Name() string                                    { return "auto.reset" }
func (c *autoReset) ParseParams(params map[string]interface{}) error { return nil }

func (c *autoReset) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if err := comm.MarkTaskToRestart(ctx, td); err != nil {
		return errors.Wrapf(err, "marking task %s to restart", conf.Task.Id)
	}
	logger.Task().Info("Task is marked as retryable, will be automatically rescheduled.")

	return nil
}
