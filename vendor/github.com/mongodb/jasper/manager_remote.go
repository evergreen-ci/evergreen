package jasper

import (
	"context"

	"github.com/mongodb/jasper/options"
)

type remoteOverrideMgr struct {
	Manager
	remote *options.Remote
}

// NewRemoteManager builds a remote manager that wraps an existing
// manager, but creates all commands with the specified remote
// options. Commands and processes with non-nil remote options will
// run over SSH.
func NewRemoteManager(m Manager, remote *options.Remote) Manager {
	return &remoteOverrideMgr{
		Manager: m,
		remote:  remote,
	}
}

func (m *remoteOverrideMgr) CreateProcess(ctx context.Context, opts *options.Create) (Process, error) {
	opts.Remote = m.remote
	return m.Manager.CreateProcess(ctx, opts)
}

func (m *remoteOverrideMgr) CreateCommand(ctx context.Context) *Command {
	cmd := m.Manager.CreateCommand(ctx)
	cmd.opts.Process.Remote = m.remote
	return cmd
}
