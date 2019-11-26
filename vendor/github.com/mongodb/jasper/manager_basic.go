package jasper

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type basicProcessManager struct {
	id            string
	procs         map[string]Process
	senv          map[string]ScriptingEnvironment
	blocking      bool
	useSSHLibrary bool
	tracker       ProcessTracker
}

func newBasicProcessManager(procs map[string]Process, blocking bool, trackProcs bool, useSSHLibrary bool) (Manager, error) {
	m := basicProcessManager{
		procs:         procs,
		senv:          make(map[string]ScriptingEnvironment),
		blocking:      blocking,
		id:            uuid.Must(uuid.NewV4()).String(),
		useSSHLibrary: useSSHLibrary,
	}
	if trackProcs {
		tracker, err := NewProcessTracker(m.id)
		if err != nil {
			return nil, errors.Wrap(err, "failed to make process tracker")
		}
		m.tracker = tracker
	}
	return &m, nil
}

func (m *basicProcessManager) ID() string {
	return m.id
}

func (m *basicProcessManager) CreateScripting(ctx context.Context, opts options.ScriptingEnvironment) (ScriptingEnvironment, error) {
	if se, ok := m.senv[opts.ID()]; ok {
		return se, nil
	}

	se, err := scriptingEnvironmentFactory(m, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err = se.Setup(ctx); err != nil {
		return nil, errors.Wrap(err, "problem setting up scripting environment")
	}

	return se, nil
}

func (m *basicProcessManager) GetScripting(ctx context.Context, id string) (ScriptingEnvironment, error) {
	se, ok := m.senv[id]
	if !ok {
		return nil, errors.Errorf("could not find scripting environment named '%s'", id)
	}
	return se, nil
}

func (m *basicProcessManager) CreateProcess(ctx context.Context, opts *options.Create) (Process, error) {
	opts.AddEnvVar(ManagerEnvironID, m.id)

	var (
		proc Process
		err  error
	)

	if opts.Remote != nil && m.useSSHLibrary {
		opts.Remote.UseSSHLibrary = true
	}
	if m.blocking {
		proc, err = newBlockingProcess(ctx, opts)
	} else {
		proc, err = newBasicProcess(ctx, opts)
	}

	if err != nil {
		return nil, errors.Wrap(err, "problem constructing process")
	}

	// This trigger is not guaranteed to be registered since the process may
	// have already completed. One way to guarantee it runs could be to add this
	// as a closer to CreateOptions.
	_ = proc.RegisterTrigger(ctx, makeDefaultTrigger(ctx, m, opts, proc.ID()))

	if m.tracker != nil {
		// The process may have terminated already, so don't return on error.
		if err := m.tracker.Add(proc.Info(ctx)); err != nil {
			grip.Warning(message.WrapError(err, "problem adding process to tracker during process creation"))
		}
	}

	m.procs[proc.ID()] = proc

	return proc, nil
}

func (m *basicProcessManager) CreateCommand(ctx context.Context) *Command {
	return NewCommand().ProcConstructor(m.CreateProcess)
}

func (m *basicProcessManager) WriteFile(ctx context.Context, opts options.WriteFile) error {
	if err := opts.Validate(); err != nil {
		return errors.Wrap(err, "invalid file options")
	}

	return errors.Wrap(opts.DoWrite(), "problem writing data")
}

func (m *basicProcessManager) Register(ctx context.Context, proc Process) error {
	if ctx.Err() != nil {
		return errors.WithStack(ctx.Err())
	}

	if proc == nil {
		return errors.New("process is not defined")
	}

	id := proc.ID()
	if id == "" {
		return errors.New("process is malformed")
	}

	if m.tracker != nil {
		// The process may have terminated already, so don't return on error.
		if err := m.tracker.Add(proc.Info(ctx)); err != nil {
			grip.Warning(message.WrapError(err, "problem adding process to tracker during process registration"))
		}
	}

	_, ok := m.procs[id]
	if ok {
		return errors.New("cannot register process that exists")
	}

	m.procs[id] = proc
	return nil
}

func (m *basicProcessManager) List(ctx context.Context, f options.Filter) ([]Process, error) {
	out := []Process{}

	if err := f.Validate(); err != nil {
		return out, errors.Wrap(err, "invalid filter")
	}

	for _, proc := range m.procs {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

		cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		info := proc.Info(cctx)
		cancel()
		switch {
		case f == options.Running:
			if info.IsRunning {
				out = append(out, proc)
			}
		case f == options.Terminated:
			if !info.IsRunning {
				out = append(out, proc)
			}
		case f == options.Successful:
			if info.Successful {
				out = append(out, proc)
			}
		case f == options.Failed:
			if info.Complete && !info.Successful {
				out = append(out, proc)
			}
		case f == options.All:
			out = append(out, proc)
		}
	}

	return out, nil
}

func (m *basicProcessManager) Get(ctx context.Context, id string) (Process, error) {
	proc, ok := m.procs[id]
	if !ok {
		return nil, errors.Errorf("process '%s' does not exist", id)
	}

	return proc, nil
}

func (m *basicProcessManager) Clear(ctx context.Context) {
	for procID, proc := range m.procs {
		if proc.Complete(ctx) {
			delete(m.procs, procID)
		}
	}
}

func (m *basicProcessManager) Close(ctx context.Context) error {
	if len(m.procs) == 0 {
		return nil
	}
	procs, err := m.List(ctx, options.Running)
	if err != nil {
		return errors.WithStack(err)
	}

	termCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if m.tracker != nil {
		if err := m.tracker.Cleanup(); err != nil {
			grip.Warning(message.WrapError(err, "process tracker did not clean up all processes successfully"))
		} else {
			return nil
		}
	}
	if err := TerminateAll(termCtx, procs); err != nil {
		killCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		return errors.WithStack(KillAll(killCtx, procs))
	}

	return nil
}

func (m *basicProcessManager) Group(ctx context.Context, name string) ([]Process, error) {
	out := []Process{}
	for _, proc := range m.procs {
		if ctx.Err() != nil {
			return nil, errors.WithStack(ctx.Err())
		}

	addTag:
		for _, t := range proc.GetTags() {
			if t == name {
				out = append(out, proc)
				break addTag
			}
		}
	}

	return out, nil
}
