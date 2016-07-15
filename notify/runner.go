package notify

import (
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
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
	evergreen.Logger.Logf(slogger.INFO, "Starting notifications at time %v", startTime)

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
