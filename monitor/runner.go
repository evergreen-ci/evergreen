package monitor

import (
	"context"

	"github.com/evergreen-ci/evergreen"
)

type Runner struct{}

const (
	// monitor "tracks and cleans up expired hosts and tasks"
	RunnerName = "monitor"
)

func (r *Runner) Name() string { return RunnerName }

func (r *Runner) Run(ctx context.Context, config *evergreen.Settings) error {
	return nil
}
