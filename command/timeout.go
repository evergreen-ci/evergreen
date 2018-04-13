package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// timeout dynamically set a task's idle or exec timeout at task runtime.
type timeout struct {
	TimeoutSecs     int `mapstructure:"timeout_secs"`
	ExecTimeoutSecs int `mapstructure:"exec_timeout_secs"`

	base
}

func timeoutUpdateFactory() Command { return &timeout{} }
func (c *timeout) Name() string     { return "timeout.update" }

// ParseParams parses the params into the the timeout struct.
func (c *timeout) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, c)
	if err != nil {
		return errors.Wrap(err, "error decoding params")
	}
	if c.TimeoutSecs == 0 && c.ExecTimeoutSecs == 0 {
		return errors.New("Must set at least one timeout")
	}

	return nil
}

// Execute updates the idle timeout.
func (c *timeout) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	if c.TimeoutSecs != 0 {
		conf.SetIdleTimeout(c.TimeoutSecs)
		logger.Execution().Infof("Set idle timeout to %d seconds", c.TimeoutSecs)
	}
	if c.ExecTimeoutSecs != 0 {
		conf.SetExecTimeout(c.ExecTimeoutSecs)
		logger.Execution().Infof("Set exec timeout to %d seconds", c.ExecTimeoutSecs)
	}
	return nil

}
