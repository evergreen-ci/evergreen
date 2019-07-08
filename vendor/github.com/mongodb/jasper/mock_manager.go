package jasper

import (
	"context"
	"runtime"

	"github.com/pkg/errors"
)

// MockManager implements the Manager interface with exported fields to
// configure and introspect the mock's behavior.
type MockManager struct {
	FailCreate   bool
	Create       func(*CreateOptions) MockProcess
	CreateConfig MockProcess
	FailRegister bool
	FailList     bool
	FailGroup    bool
	FailGet      bool
	FailClose    bool
	Procs        []Process
}

func mockFail() error {
	progCounter := make([]uintptr, 2)
	n := runtime.Callers(2, progCounter)
	frames := runtime.CallersFrames(progCounter[:n])
	frame, _ := frames.Next()
	return errors.Errorf("function failed: %s", frame.Function)
}

// CreateProcess creates a new MockProcess. If Create is set, it is
// invoked to create the MockProcess. Otherwise, CreateConfig is used as a
// template to create the MockProcess. The new MockProcess is put in Procs. If
// FailCreate is set, it returns an error.
func (m *MockManager) CreateProcess(ctx context.Context, opts *CreateOptions) (Process, error) {
	if m.FailCreate {
		return nil, mockFail()
	}

	var proc MockProcess
	if m.Create != nil {
		proc = m.Create(opts)
	} else {
		proc = MockProcess(m.CreateConfig)
		proc.ProcInfo.Options = *opts
	}

	m.Procs = append(m.Procs, &proc)

	return &proc, nil
}

// CreateCommand creates a Command that invokes CreateProcess to create the
// underlying processes.
func (m *MockManager) CreateCommand(ctx context.Context) *Command {
	return NewCommand().ProcConstructor(m.CreateProcess)
}

// Register adds the process to Procs. If FailRegister is set, it returns an
// error.
func (m *MockManager) Register(ctx context.Context, proc Process) error {
	if m.FailRegister {
		return mockFail()
	}

	m.Procs = append(m.Procs, proc)

	return nil
}

// List returns all processes that match the given filter. If FailList is set,
// it returns an error.
func (m *MockManager) List(ctx context.Context, f Filter) ([]Process, error) {
	if m.FailList {
		return nil, mockFail()
	}

	filteredProcs := []Process{}

	for _, proc := range m.Procs {
		info := proc.Info(ctx)
		switch f {
		case All:
			filteredProcs = append(filteredProcs, proc)
		case Running:
			if info.IsRunning {
				filteredProcs = append(filteredProcs, proc)
			}
		case Terminated:
			if !info.IsRunning {
				filteredProcs = append(filteredProcs, proc)
			}
		case Failed:
			if info.Complete && !info.Successful {
				filteredProcs = append(filteredProcs, proc)
			}
		case Successful:
			if info.Successful {
				filteredProcs = append(filteredProcs, proc)
			}
		default:
			return nil, errors.Errorf("invalid filter '%s'", f)
		}
	}

	return filteredProcs, nil
}

// Group returns all processses that have the given tag. If FailGroup is set, it
// returns an error.
func (m *MockManager) Group(ctx context.Context, tag string) ([]Process, error) {
	if m.FailGroup {
		return nil, mockFail()
	}

	matchingProcs := []Process{}
	for _, proc := range m.Procs {
		for _, procTag := range proc.GetTags() {
			if procTag == tag {
				matchingProcs = append(matchingProcs, proc)
			}
		}
	}

	return matchingProcs, nil
}

// Get returns a process given by ID from Procs. If a matching process is not
// found in Procs or if FailGet is set, it returns an error.
func (m *MockManager) Get(ctx context.Context, id string) (Process, error) {
	if m.FailGet {
		return nil, mockFail()
	}

	for _, proc := range m.Procs {
		if proc.ID() == id {
			return proc, nil
		}
	}

	return nil, errors.Errorf("proc with id '%s' not found", id)
}

// Clear removes all processes from Procs.
func (m *MockManager) Clear(ctx context.Context) {
	m.Procs = []Process{}
}

// Close clears all processes in Procs. If FailClose is set, it returns an
// error.
func (m *MockManager) Close(ctx context.Context) error {
	if m.FailClose {
		return mockFail()
	}
	m.Clear(ctx)
	return nil
}
