package notify

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

type Runner struct{}

const (
	// "the notify send notifications for and system issues"
	RunnerName = "notify"
)

func (r *Runner) Name() string { return RunnerName }

func (r *Runner) Run(ctx context.Context, config *evergreen.Settings) error {
	startTime := time.Now()
	adminSettings, err := admin.GetSettings()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.NotificationsDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  RunnerName,
			"message": "notify is disabled, exiting",
		})
		return nil
	}

	grip.Info(message.Fields{
		"runner":  RunnerName,
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	if err := Run(config); err != nil {
		grip.Error(message.Fields{
			"runner":  RunnerName,
			"error":   err.Error(),
			"status":  "failed",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
		})

		return errors.Wrap(err, "problem running notify")
	}

	runspan := time.Since(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runspan); err != nil {
		grip.Error(message.WrapErrors(err, message.Fields{
			"runner":   RunnerName,
			"message":  "problem setting runner complete",
			"duration": runspan,
			"span":     runspan.String(),
		}))
	}

	grip.Info(message.Fields{
		"runner":    r.Name(),
		"runtime":   time.Since(startTime),
		"operation": "success",
		"span":      time.Since(startTime).String(),
	})

	return nil
}
