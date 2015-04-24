package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

type Runner struct{}

const (
	RunnerName = "monitor"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *mci.MCISettings) error {
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "error acquiring global lock: %v", err)
	}

	if !lockAcquired {
		return mci.Logger.Errorf(slogger.ERROR, "timed out acquiring global lock")
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting monitor at time %v", startTime)

	if err := RunAllMonitoring(config); err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "error running monitor: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Monitor took %v to run", runtime)
	return nil
}
