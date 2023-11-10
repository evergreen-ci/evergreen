package command

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mongodb/jasper"
)

// Command is an interface that defines a command
// A Command takes parameters as a map, and is executed after
// those parameters are parsed.
type Command interface {
	// ParseParams takes a map of fields to values extracted from
	// the project config and passes them to the command. Any
	// errors parsing the information are returned.
	ParseParams(map[string]interface{}) error

	// Execute runs the command using the agent's logger, communicator,
	// task config, and a channel for interrupting long-running commands.
	// Execute is called after ParseParams.
	Execute(context.Context, client.Communicator, client.LoggerProducer, *internal.TaskConfig) error

	// Name is the name of the command.
	Name() string

	// Type returns the command's type (e.g. system or test).
	Type() string
	// SetType sets the command's type (e.g. system or test).
	SetType(string)

	// FullDisplayName is the full display name for the command. The full
	// command name includes the command name (including the type of command and
	// the user-defined display name if any) as well as other relevant context
	// like the function and block the command runs in.
	FullDisplayName() string
	SetFullDisplayName(string)

	// IdleTimeout is the user-configurable timeout for how long an individual
	// command can run without writing output to the task logs. If the command
	// hits this timeout, then it will time out and stop early.
	// This timeout only applies in certain blocks, such as pre, setup group,
	// setup task, and the main task block.
	IdleTimeout() time.Duration
	SetIdleTimeout(time.Duration)

	// JasperManager is the Jasper process manager for the command. Jasper can
	// be used to run and manage processes that are started within commands.
	JasperManager() jasper.Manager
	SetJasperManager(jasper.Manager)
}

// base contains a basic implementation of functionality that is
// common to all command implementations.
type base struct {
	idleTimeout     time.Duration
	typeName        string
	fullDisplayName string
	jasper          jasper.Manager
	mu              sync.RWMutex
}

func (b *base) Type() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.typeName
}

func (b *base) SetType(n string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.typeName = n
}

func (b *base) FullDisplayName() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.fullDisplayName
}

func (b *base) SetFullDisplayName(n string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.fullDisplayName = n
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
