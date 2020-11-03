package command

import (
	"context"
	"strconv"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// timeout dynamically set a task's idle or exec timeout at task runtime.
type timeout struct {
	TimeoutSecs     int `mapstructure:"timeout_secs"`
	ExecTimeoutSecs int `mapstructure:"exec_timeout_secs"`

	params map[string]interface{}

	base
}

// timeout dynamically set a task's idle or exec timeout at task runtime.
type timeoutStr struct {
	TimeoutSecs     string `mapstructure:"timeout_secs" plugin:"expand"`
	ExecTimeoutSecs string `mapstructure:"exec_timeout_secs" plugin:"expand"`

	base
}

func timeoutUpdateFactory() Command { return &timeout{} }
func (c *timeout) Name() string     { return "timeout.update" }

// ParseParams parses the params into the the timeout struct.
func (c *timeout) ParseParams(params map[string]interface{}) error {
	c.params = params
	return nil
}

// Execute updates the idle timeout.
func (c *timeout) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {
	// do the params parsing here rather than in ParseParams because we want
	// to destructure only if parsing as ints fails.
	if err := mapstructure.Decode(c.params, c); err != nil ||
		(c.TimeoutSecs == 0 && c.ExecTimeoutSecs == 0) {
		// If destructuring as ints fails, or neither timeout is set, destructure as strings.
		t := &timeoutStr{}
		if errStr := mapstructure.Decode(c.params, t); errStr != nil {
			return errors.New("could not decode params as either string or int")
		}
		if err := util.ExpandValues(t, conf.Expansions); err != nil {
			return errors.Wrap(err, "error expanding expansion values")
		}
		timeout, errTimeout := strconv.Atoi(t.TimeoutSecs)
		exec, errExec := strconv.Atoi(t.ExecTimeoutSecs)
		if errTimeout != nil && errExec != nil {
			return errors.Errorf("could not convert strings to int, %s, %s", t.TimeoutSecs, t.ExecTimeoutSecs)
		}
		c.TimeoutSecs = timeout
		c.ExecTimeoutSecs = exec
	}

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
