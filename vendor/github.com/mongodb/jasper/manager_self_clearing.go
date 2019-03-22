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
		localProcessManager: localManager.(*localProcessManager),
		maxProcs:            maxProcs,
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
		localProcessManager: localBlockingManager.(*localProcessManager),
		maxProcs:            maxProcs,
	}, nil
}

type selfClearingProcessManager struct {
	*localProcessManager
	maxProcs int
}

func (m *selfClearingProcessManager) checkProcCapacity(ctx context.Context) error {
	if len(m.manager.procs) == m.maxProcs {
		// We are at capacity, we can try to perform a clear.
		m.Clear(ctx)
		if len(m.manager.procs) == m.maxProcs {
			return errors.New("cannot create any more processes")
		}
	}

	return nil
}

func (m *selfClearingProcessManager) CreateProcess(ctx context.Context, opts *CreateOptions) (Process, error) {
	if err := m.checkProcCapacity(ctx); err != nil {
		return nil, err
	}

	proc, err := m.localProcessManager.CreateProcess(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return proc, nil
}

func (m *selfClearingProcessManager) Register(ctx context.Context, proc Process) error {
	if err := m.checkProcCapacity(ctx); err != nil {
		return err
	}

	return errors.WithStack(m.localProcessManager.Register(ctx, proc))
}
