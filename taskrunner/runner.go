package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

type Runner struct{}

const (
	RunnerName = "taskrunner"
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
		err := db.ReleaseGlobalLock(RunnerName)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting taskrunner at time %v", startTime)

	if err = NewTaskRunner(config).Run(); err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "error running taskrunner: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Taskrunner took %v to run", runtime)
	return nil
}
