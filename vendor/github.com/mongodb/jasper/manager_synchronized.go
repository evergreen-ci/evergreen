package jasper

import (
	"context"
	"sync"

	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// MakeSynchronizedManager wraps the given manager in a thread-safe Manager.
func MakeSynchronizedManager(manager Manager) (Manager, error) {
	return &synchronizedProcessManager{manager: manager}, nil
}

// NewSynchronizedManager is a constructor for a thread-safe Manager.
func NewSynchronizedManager(trackProcs bool) (Manager, error) {
	basicManager, err := newBasicProcessManager(map[string]Process{}, false, trackProcs)
	if err != nil {
		return nil, err
	}

	return &synchronizedProcessManager{manager: basicManager}, nil
}

// NewSynchronizedManagerBlockingProcesses is a constructor for a thread-safe Manager
// that uses blockingProcess instead of the default basicProcess.
func NewSynchronizedManagerBlockingProcesses(trackProcs bool) (Manager, error) {
	basicBlockingManager, err := newBasicProcessManager(map[string]Process{}, true, trackProcs)
	if err != nil {
		return nil, err
	}

	return &synchronizedProcessManager{manager: basicBlockingManager}, nil
}

type synchronizedProcessManager struct {
	mu      sync.RWMutex
	manager Manager
}

func (m *synchronizedProcessManager) ID() string {
	return m.manager.ID()
}

func (m *synchronizedProcessManager) CreateProcess(ctx context.Context, opts *options.Create) (Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	proc, err := m.manager.CreateProcess(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &synchronizedProcess{proc: proc}, nil
}

func (m *synchronizedProcessManager) CreateCommand(ctx context.Context) *Command {
	return NewCommand().ProcConstructor(m.CreateProcess)
}

func (m *synchronizedProcessManager) Register(ctx context.Context, proc Process) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return errors.WithStack(m.manager.Register(ctx, proc))
}

func (m *synchronizedProcessManager) List(ctx context.Context, f options.Filter) ([]Process, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	procs, err := m.manager.List(ctx, f)
	return procs, errors.WithStack(err)
}

func (m *synchronizedProcessManager) Get(ctx context.Context, id string) (Process, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	proc, err := m.manager.Get(ctx, id)
	return proc, errors.WithStack(err)
}

func (m *synchronizedProcessManager) Clear(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.manager.Clear(ctx)
}

func (m *synchronizedProcessManager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return errors.WithStack(m.manager.Close(ctx))
}

func (m *synchronizedProcessManager) Group(ctx context.Context, name string) ([]Process, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	procs, err := m.manager.Group(ctx, name)
	return procs, errors.WithStack(err)
}
