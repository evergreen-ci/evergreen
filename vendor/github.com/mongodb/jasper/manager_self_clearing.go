package jasper

import (
	"context"

	"github.com/pkg/errors"
)

type selfClearingProcessManager struct {
	*basicProcessManager
	maxProcs int
}

// NewSelfClearingProcessManager creates and returns a process manager that
// places a limit on the number of concurrent processes stored by Jasper at any
// given time, and will clear itself of dead processes when necessary without
// the need for calling Clear() from the user. Clear() can, however, be called
// proactively. This manager therefore gives no guarantees on the persistence
// of a process in its memory that has already completed.
//
// The self clearing process manager is not thread safe. Wrap with the
// local process manager for multithreaded use.
func NewSelfClearingProcessManager(maxProcs int, trackProcs bool) (Manager, error) {
	pm, err := newBasicProcessManager(map[string]Process{}, false, false, trackProcs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bpm, ok := pm.(*basicProcessManager)
	if !ok {
		return nil, errors.New("process manager construction error")
	}

	return &selfClearingProcessManager{
		basicProcessManager: bpm,
		maxProcs:            maxProcs,
	}, nil
}

// NewSelfClearingProcessManagerBlockingProcesses creates and returns a process
// manager that uses blockingProcesses rather than the default basicProcess.
// See the NewSelfClearingProcessManager() constructor for more information.
//
// The self clearing process manager is not thread safe. Wrap with the
// local process manager for multithreaded use.
func NewSelfClearingProcessManagerBlockingProcesses(maxProcs int, trackProcs bool) (Manager, error) {
	pm, err := newBasicProcessManager(map[string]Process{}, false, true, trackProcs)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bpm, ok := pm.(*basicProcessManager)
	if !ok {
		return nil, errors.New("process manager construction error")
	}
	return &selfClearingProcessManager{
		basicProcessManager: bpm,
		maxProcs:            maxProcs,
	}, nil
}

func (m *selfClearingProcessManager) checkProcCapacity(ctx context.Context) error {
	if len(m.basicProcessManager.procs) == m.maxProcs {
		// We are at capacity, we can try to perform a clear.
		m.Clear(ctx)
		if len(m.basicProcessManager.procs) == m.maxProcs {
			return errors.New("cannot create any more processes")
		}
	}

	return nil
}

func (m *selfClearingProcessManager) CreateProcess(ctx context.Context, opts *CreateOptions) (Process, error) {
	if err := m.checkProcCapacity(ctx); err != nil {
		return nil, err
	}

	proc, err := m.basicProcessManager.CreateProcess(ctx, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return proc, nil
}

func (m *selfClearingProcessManager) Register(ctx context.Context, proc Process) error {
	if err := m.checkProcCapacity(ctx); err != nil {
		return err
	}

	return errors.WithStack(m.basicProcessManager.Register(ctx, proc))
}
