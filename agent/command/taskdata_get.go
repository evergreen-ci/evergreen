package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
)

// TODO EVG-17643: remove json.get and json.get_history

type taskDataHistory struct {
	base
}

func taskDataHistoryFactory() Command   { return &taskDataHistory{} }
func (c *taskDataHistory) Name() string { return "json.get_history" }

func (c *taskDataHistory) ParseParams(params map[string]interface{}) error {
	return nil
}

func (c *taskDataHistory) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	return nil
}
