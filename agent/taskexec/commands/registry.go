package commands

import (
	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/pkg/errors"
)

// LocalCommand represents a command that can be executed locally.
type LocalCommand interface {
	command.Command
	// LocalExecute runs the command in a local execution context
	LocalExecute() error
}

// CommandFactory creates a new instance of a command.
type CommandFactory func() command.Command

// Registry provides access to the global command registry.
type Registry struct{}

// NewRegistry creates a new registry instance.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterCommand registers a command factory with the global registry.
func (r *Registry) RegisterCommand(name string, factory CommandFactory) error {
	if name == "" {
		return errors.New("cannot register a command without a name")
	}
	if factory == nil {
		return errors.Errorf("cannot register a nil factory for command '%s'", name)
	}
	return command.RegisterCommand(name, command.CommandFactory(factory))
}

// GetCommandFactory retrieves a command factory from the global registry.
func (r *Registry) GetCommandFactory(name string) (CommandFactory, bool) {
	factory, ok := command.GetCommandFactory(name)
	if !ok {
		return nil, false
	}
	return CommandFactory(factory), true
}

// RegisteredCommandNames returns all registered command names from the global registry.
func (r *Registry) RegisteredCommandNames() []string {
	return command.RegisteredCommandNames()
}
