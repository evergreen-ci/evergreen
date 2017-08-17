package notify

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type Runner struct{}

const (
	RunnerName  = "notify"
	Description = "send notifications for failed tasks and system issues"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	adminSettings, err := admin.GetSettingsFromDB()
	if err != nil {
		grip.Error(errors.Wrap(err, "error retrieving admin settings"))
	}
	if adminSettings != nil && adminSettings.ServiceFlags.NotificationsDisabled {
		grip.Info(message.Fields{
			"runner":  RunnerName,
			"status":  "success",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
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

	if err := model.SetProcessRuntimeCompleted(RunnerName, time.Since(startTime)); err != nil {
		grip.Error(errors.Wrap(err, "problem updating process status"))
	}

	grip.Info(message.Fields{
		"runner":    r.Name(),
		"runtime":   time.Since(startTime),
		"operation": "success",
		"span":      time.Since(startTime).String(),
	})

	return nil
}
