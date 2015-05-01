package scheduler

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"time"
)

type Runner struct{}

const (
	RunnerName = "scheduler"
)

func (r *Runner) Name() string {
	return RunnerName
}

func (r *Runner) Run(config *evergreen.MCISettings) error {
	lockAcquired, err := db.WaitTillAcquireGlobalLock(RunnerName, db.LockTimeout)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
	}

	if !lockAcquired {
		return evergreen.Logger.Errorf(slogger.ERROR, "Timed out acquiring global lock")
	}

	defer func() {
		if err := db.ReleaseGlobalLock(RunnerName); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error releasing global lock: %v", err)
		}
	}()

	startTime := time.Now()
	evergreen.Logger.Logf(slogger.INFO, "Starting scheduler at time %v", startTime)

	schedulerInstance := &Scheduler{
		config,
		&DBTaskFinder{},
		NewCmpBasedTaskPrioritizer(),
		&DBTaskDurationEstimator{},
		&DBTaskQueuePersister{},
		&DurationBasedHostAllocator{},
	}

	if err = schedulerInstance.Schedule(); err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Error running scheduler: %v", err)
	}

	runtime := time.Now().Sub(startTime)
	if err = model.SetProcessRuntimeCompleted(RunnerName, runtime); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Scheduler took %v to run", runtime)
	return nil
}
