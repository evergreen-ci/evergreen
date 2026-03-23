//go:build !linux

package agent

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
)

func (a *Agent) maybeStartContainer(_ context.Context, _ *internal.TaskConfig) error {
	return nil
}

func (a *Agent) destroyContainer(_ context.Context, _ *internal.TaskConfig) {}
