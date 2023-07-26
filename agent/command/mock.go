package command

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

type mockCommand struct {
	SleepSeconds int `mapstructure:"sleep_seconds"`

	base
}

// MockCommandFactory is a factory used to produce a mock command for testing.
// should not be used in production.
func MockCommandFactory() Command { return &mockCommand{} }

// Execute sleeps for the given duration if it is specified. Otherwise, it is a
// no-op.
func (c *mockCommand) Execute(context.Context, client.Communicator, client.LoggerProducer, *internal.TaskConfig) error {
	if c.SleepSeconds != 0 {
		// kim: NOTE: this ignores the context, which is good for the test
		time.Sleep(time.Duration(c.SleepSeconds) * time.Second)
	}
	return nil
}

// Name returns the value of the command's name.
func (c *mockCommand) Name() string { return "command.mock" }

// ParseParams parses the parameters to the mock command, if any.
func (c *mockCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}
	return nil
}
