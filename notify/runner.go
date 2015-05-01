package notify

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"time"
)

type Runner struct{}

const (
	RunnerName = "notify"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *evergreen.Settings) error {
	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Starting notifications at time %v", startTime)
	evergreen.Logger.Logf(slogger.INFO, "Running notifications with db %v and notifications configuration %v/%v", config.Db, config.ConfigDir, evergreen.NotificationsFile)

	if err := Run(config); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "error running notify: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Notify took %v to run", runtime)
	return nil
}
