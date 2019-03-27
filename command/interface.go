package command

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/jasper"
)

// Command is an interface that defines a command
// A Command takes parameters as a map, and is executed after
// those parameters are parsed.
type Command interface {
	// ParseParams takes a map of fields to values extracted from
	// the project config and passes them to the command. Any
	// errors parsing the information are returned.
	ParseParams(params map[string]interface{}) error

	// Execute runs the command using the agent's logger, communicator,
	// task config, and a channel for interrupting long-running commands.
	// Execute is called after ParseParams.
	Execute(context.Context, client.Communicator, client.LoggerProducer, *model.TaskConfig) error

	// A string name for the command
	Name() string

	// Type reports on or overrides the default command type
	// (e.g. system or task.) The setter MUST NOT override a value
	// if it has already been set.
	Type() string
	SetType(string)

	DisplayName() string
	SetDisplayName(string)

	IdleTimeout() time.Duration
	SetIdleTimeout(time.Duration)

	SetJasperManager(jasper.Manager)
	JasperManager() jasper.Manager
}

// base contains a basic implementation of functionality that is
// common to all command implementations.
type base struct {
	idleTimeout time.Duration
	typeName    string
	displayName string
	jasper      jasper.Manager
	mu          sync.RWMutex
}

func (b *base) Type() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.typeName
}

func (b *base) SetType(n string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.typeName == "" {
		b.typeName = n
	}
}

func (b *base) DisplayName() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.displayName
}

func (b *base) SetDisplayName(n string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.displayName = n
}

func (b *base) SetIdleTimeout(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.idleTimeout = d
}

func (b *base) IdleTimeout() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.idleTimeout
}

func (b *base) SetJasperManager(jpm jasper.Manager) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.jasper = jpm
}

func (b *base) JasperManager() jasper.Manager {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.jasper
}
