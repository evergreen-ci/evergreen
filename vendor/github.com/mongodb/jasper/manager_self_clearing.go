package jasper

import (
	"context"

	"github.com/pkg/errors"
)

// NewSelfClearingProcessManager creates and returns a process manager that
// places a limit on the number of concurrent processes stored by Jasper at any
// given time, and will clear itself of dead processes when necessary without
// the need for calling Clear() from the user. Clear() can, however, be called
// proactively. This manager therefore gives no guarantees on the persistence
// of a process in its memory that has already completed.
func NewSelfClearingProcessManager(maxProcs int, trackProcs bool) (Manager, error) {
	localManager, err := NewLocalManager(trackProcs)
	if err != nil {
		return nil, err
	}
	return &selfClearingProcessManager{
		local:    localManager.(*localProcessManager),
		maxProcs: maxProcs,
	}, nil
}

// NewSelfClearingProcessManagerBlockingProcesses creates and returns a process
// manager that uses blockingProcesses rather than the default basicProcess.
// See the NewSelfClearingProcessManager() constructor for more information.
func NewSelfClearingProcessManagerBlockingProcesses(maxProcs int, trackProcs bool) (Manager, error) {
	localBlockingManager, err := NewLocalManagerBlockingProcesses(trackProcs)
	if err != nil {
		return nil, err
	}
	return &selfClearingProcessManager{
		local:    localBlockingManager.(*localProcessManager),
		maxProcs: maxProcs,
	}, nil
}

type selfClearingProcessManager struct {
	local    *localProcessManager
	maxProcs int
}

func (m *selfClearingProcessManager) checkProcCapacity(ctx context.Context) error {
	if len(m.local.manager.procs) == m.maxProcs {
		// We are at capacity, we can try to perform a clear.
		m.Clear(ctx)
		if len(m.local.manager.procs) == m.maxProcs {
			return errors.New("cannot create any more processes")
		}
	}

	return nil
}

func (m *selfClearingProcessManager) CreateProcess(ctx context.Context, opts *CreateOptions) (Process, error) {
	if err := m.checkProcCapacity(ctx); err != nil {
		return nil, err
	}

	proc, err := m.local.CreateProcess(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return proc, nil
}

func (m *selfClearingProcessManager) CreateCommand(ctx context.Context) *Command {
	return NewCommand().ProcConstructor(m.CreateProcess)
}

func (m *selfClearingProcessManager) Register(ctx context.Context, proc Process) error {
	if err := m.checkProcCapacity(ctx); err != nil {
		return err
	}

	return errors.WithStack(m.local.Register(ctx, proc))
}

func (m *selfClearingProcessManager) List(ctx context.Context, f Filter) ([]Process, error) {
	procs, err := m.local.List(ctx, f)
	return procs, errors.WithStack(err)
}

func (m *selfClearingProcessManager) Get(ctx context.Context, id string) (Process, error) {
	proc, err := m.local.Get(ctx, id)
	return proc, errors.WithStack(err)
}

func (m *selfClearingProcessManager) Clear(ctx context.Context) {
	m.local.Clear(ctx)
}

func (m *selfClearingProcessManager) Close(ctx context.Context) error {
	return errors.WithStack(m.local.Close(ctx))
}

func (m *selfClearingProcessManager) Group(ctx context.Context, name string) ([]Process, error) {
	procs, err := m.local.Group(ctx, name)
	return procs, errors.WithStack(err)
}
