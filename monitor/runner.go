package monitor

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

type Runner struct{}

const (
	// monitor "tracks and cleans up expired hosts and tasks"
	RunnerName = "monitor"
)

func (r *Runner) Name() string { return RunnerName }

func (r *Runner) Run(ctx context.Context, config *evergreen.Settings) error {
	startTime := time.Now()
	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	if err := RunAllMonitoring(ctx, config); err != nil {
		grip.Error(message.Fields{
			"runner":  RunnerName,
			"error":   err.Error(),
			"status":  "failed",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
		})

		return errors.Wrap(err, "problem running monitor")
	}

	if err := model.SetProcessRuntimeCompleted(RunnerName, time.Since(startTime)); err != nil {
		grip.Error(errors.Wrap(err, "problem updating process status"))
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"runtime": time.Since(startTime),
		"status":  "success",
		"span":    time.Since(startTime).String(),
	})

	return nil
}
