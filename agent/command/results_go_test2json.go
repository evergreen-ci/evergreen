package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
)

type goTest2JSONCommand struct {
	Files []string `mapstructure:"files" plugin:"expand"`

	base
}

// gotest.parse_json is deprecated
func goTest2JSONFactory() Command                                             { return &goTest2JSONCommand{} }
func (c *goTest2JSONCommand) Name() string                                    { return "gotest.parse_json" }
func (c *goTest2JSONCommand) ParseParams(params map[string]interface{}) error { return nil }
func (c *goTest2JSONCommand) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	logger.Task().Warning("gotest.parse_json deprecated")
	return nil
}
