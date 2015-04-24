package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

type Runner struct{}

const (
	RunnerName = "scheduler"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *mci.MCISettings) error {
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
	}

	if !lockAcquired {
		return mci.Logger.Errorf(slogger.ERROR, "Timed out acquiring global lock")
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting scheduler at time %v", startTime)

	schedulerInstance := &Scheduler{
		config,
		&DBTaskFinder{},
		NewCmpBasedTaskPrioritizer(),
		&DBTaskDurationEstimator{},
		&DBTaskQueuePersister{},
		&DurationBasedHostAllocator{},
	}

	if err = schedulerInstance.Schedule(); err != nil {
		return mci.Logger.Errorf(slogger.ERROR, "Error running scheduler: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Scheduler took %v to run", runtime)
	return nil
}
