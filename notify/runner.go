package notify

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

type Runner struct{}

const (
	RunnerName = "notify"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *mci.MCISettings) error {
	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting notifications at time %v", startTime)
	mci.Logger.Logf(slogger.INFO, "Running notifications with db %v and notifications configuration %v/%v", config.Db, config.ConfigDir, mci.NotificationsFile)

	if err := Run(config); err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "error running notify: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Notify took %v to run", runtime)
	return nil
}
