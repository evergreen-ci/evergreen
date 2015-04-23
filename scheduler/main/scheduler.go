package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/scheduler"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

const (
	SchedulerLockTitle = "scheduler"
)

// Main function for the scheduler package.  Creates a Scheduler instance,
// and calls its Schedule() function.
func main() {

	// load in and set appropriate configuration
	mciSettings := mci.MustConfig()
	if mciSettings.Scheduler.LogFile != "" {
		mci.SetLogger(mciSettings.Scheduler.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	// take the global lock before doing anything
	lockAcquired, err := db.WaitTillAcquireGlobalLock(SchedulerLockTitle,
		db.LockTimeout)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
		return
	}
	if !lockAcquired {
		mci.Logger.Logf(slogger.WARN, "Cannot proceed with scheduler because"+
			" the global lock could not be taken")
		return
	}

	// defer releasing the global lock
	defer func() {
		err := db.ReleaseGlobalLock(SchedulerLockTitle)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error releasing global lock"+
				" from scheduler - this is really bad: %v", err)
		}
	}()

	startTime := time.Now()

	schedulerInstance := &scheduler.Scheduler{
		mciSettings,
		&scheduler.DBTaskFinder{},
		scheduler.NewCmpBasedTaskPrioritizer(),
		&scheduler.DBTaskDurationEstimator{},
		&scheduler.DBTaskQueuePersister{},
		&scheduler.DurationBasedHostAllocator{},
	}

	err = schedulerInstance.Schedule()

	if err != nil {
		panic(fmt.Sprintf("Error running scheduler: %v", err))
	}

	// report the status
	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.SchedulerPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v",
			err)
	}

}
