package jasper

import (
	"context"

	"github.com/mongodb/jasper/options"
)

type remoteOverrideMgr struct {
	remote *options.Remote
	mgr    Manager
}

// NewRemoteManager builds a remote manager that wraps an existing
// manager, but creates all commands with the specified remote
// options. Commands and processes with non-nil remote options will
// run over SSH.
func NewRemoteManager(mgr Manager, remote *options.Remote) Manager {
	return &remoteOverrideMgr{
		remote: remote,
		mgr:    mgr,
	}
}

func (m *remoteOverrideMgr) ID() string { return m.mgr.ID() }
func (m *remoteOverrideMgr) CreateProcess(ctx context.Context, opts *options.Create) (Process, error) {
	opts.Remote = m.remote
	return m.mgr.CreateProcess(ctx, opts)
}

func (m *remoteOverrideMgr) CreateScripting(ctx context.Context, opts options.ScriptingEnvironment) (ScriptingEnvironment, error) {
	return m.mgr.CreateScripting(ctx, opts)
}

func (m *remoteOverrideMgr) GetScripting(ctx context.Context, id string) (ScriptingEnvironment, error) {
	return m.mgr.GetScripting(ctx, id)
}

func (m *remoteOverrideMgr) CreateCommand(ctx context.Context) *Command {
	cmd := m.mgr.CreateCommand(ctx)
	cmd.opts.Remote = m.remote
	return cmd
}

func (m *remoteOverrideMgr) WriteFile(ctx context.Context, opts options.WriteFile) error {
	return m.mgr.WriteFile(ctx, opts)
}

func (m *remoteOverrideMgr) Register(ctx context.Context, p Process) error {
	return m.mgr.Register(ctx, p)
}

func (m *remoteOverrideMgr) List(ctx context.Context, f options.Filter) ([]Process, error) {
	return m.mgr.List(ctx, f)
}

func (m *remoteOverrideMgr) Group(ctx context.Context, n string) ([]Process, error) {
	return m.mgr.Group(ctx, n)
}

func (m *remoteOverrideMgr) Get(ctx context.Context, n string) (Process, error) {
	return m.mgr.Get(ctx, n)
}

func (m *remoteOverrideMgr) Clear(ctx context.Context)       { m.mgr.Clear(ctx) }
func (m *remoteOverrideMgr) Close(ctx context.Context) error { return m.mgr.Close(ctx) }
