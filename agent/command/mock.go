package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
)

type mockCommand struct {
	name string

	base
}

// Execute is a noop for the mock command.
func (m *mockCommand) Execute(context.Context, client.Communicator, client.LoggerProducer, *internal.TaskConfig) error {
	return nil
}

// Name returns the value of the command's name.
func (m *mockCommand) Name() string { return m.name }

// ParseParams is a noop for the mock command.
func (m *mockCommand) ParseParams(map[string]interface{}) error { return nil }
