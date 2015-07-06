package monitor

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"time"
)

type Runner struct{}

const (
	RunnerName  = "monitor"
	Description = "track and clean up expired hosts and tasks"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Description() string {
	return Description
}

func (r *Runner) Run(config *evergreen.Settings) error {
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "error acquiring global lock: %v", err)
	}

	if !lockAcquired {
		return evergreen.Logger.Errorf(slogger.ERROR, "timed out acquiring global lock")
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Starting monitor at time %v", startTime)

	if err := RunAllMonitoring(config); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "error running monitor: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Monitor took %v to run", runtime)
	return nil
}
