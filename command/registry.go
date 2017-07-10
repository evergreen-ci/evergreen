package command

import (
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var evgRegistry *commandRegistry

func init() {
	evgRegistry = newCommandRegistry()
}

func RegisterCommand(name string, factory CommandFactory) error {
	return errors.Wrap(evgRegistry.registerCommand(name, factory),
		"problem registering command")
}

func GetCommandFactory(name string) (CommandFactory, bool) {
	return evgRegistry.getCommandFactory(name)
}

type CommandFactory func() (Command, bool)

type commandRegistry struct {
	mu   *sync.RWMutex
	cmds map[string]CommandFactory
}

func newCommandRegistry() *commandRegistry {
	return &commandRegistry{
		cmds: map[string]CommandFactory{},
		mu:   &sync.RWMutex{},
	}
}

func (r *commandRegistry) registerCommand(name string, factory CommandFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return errors.New("cannot register a command for the empty string ''")
	}

	if _, ok := r.cmds[name]; ok {
		return errors.Errorf("command '%s' is already registered", name)
	}

	if factory == nil {
		return errors.Errorf("cannot register a nil factory for command '%s'", name)
	}

	grip.Info(message.Fields{
		"message": "registering command",
		"command": name,
	})

	r.cmds[name] = factory
	return nil
}

func (r *commandRegistry) getCommandFactory(name string) (CommandFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.cmds[name]
	return factory, ok
}
