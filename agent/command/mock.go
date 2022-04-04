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

func (m *mockCommand) Execute(context.Context, client.Communicator, client.LoggerProducer, *internal.TaskConfig) error {
	return nil
}
func (m *mockCommand) Name() string                             { return m.name }
func (m *mockCommand) ParseParams(map[string]interface{}) error { return nil }
