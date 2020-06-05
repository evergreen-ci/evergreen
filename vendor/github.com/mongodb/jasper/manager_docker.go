package jasper

import (
	"context"

	"github.com/mongodb/jasper/options"
)

type dockerManager struct {
	Manager
	opts *options.Docker
}

// NewDockerManager returns a manager in which each process is created within
// a Docker container with the given options.
func NewDockerManager(m Manager, opts *options.Docker) Manager {
	return &dockerManager{
		Manager: m,
		opts:    opts,
	}
}

func (m *dockerManager) CreateProcess(ctx context.Context, opts *options.Create) (Process, error) {
	opts.Docker = m.opts
	return m.Manager.CreateProcess(ctx, opts)
}

func (m *dockerManager) CreateCommand(ctx context.Context) *Command {
	cmd := m.Manager.CreateCommand(ctx)
	cmd.opts.Process.Docker = m.opts
	return cmd
}
