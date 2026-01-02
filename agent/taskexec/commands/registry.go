package commands

import (
	"sync"

	"github.com/pkg/errors"
)

type CommandFactory func() interface{}

type CommandRegistry struct {
	mu   *sync.RWMutex
	cmds map[string]CommandFactory
}

func (r *CommandRegistry) RegisterCommand(name string, factory CommandFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return errors.New("cannot register a command without a name")
	}

	if _, ok := r.cmds[name]; ok {
		return errors.Errorf("command '%s' is already registered", name)
	}

	if factory == nil {
		return errors.Errorf("cannot register a nil factory for command '%s'", name)
	}

	r.cmds[name] = factory
	return nil
}

func (r *CommandRegistry) GetCommandFactory(name string) (CommandFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.cmds[name]
	return factory, ok
}

func (r *CommandRegistry) RegisteredCommandNames() []string {
	out := []string{}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for name := range r.cmds {
		out = append(out, name)
	}

	return out
}
