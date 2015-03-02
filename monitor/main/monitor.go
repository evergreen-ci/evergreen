package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/monitor"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

const (
	MonitorLockTitle = "monitor"
)

// main function for the monitoring package
func main() {

	// load in the config settings
	mciSettings := mci.MustConfig()
	if mciSettings.Monitor.LogFile != "" {
		mci.SetLogger(mciSettings.Monitor.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	// take the global lock before doing anything
	lockAcquired, err := db.WaitTillAcquireGlobalLock(MonitorLockTitle,
		db.LockTimeout)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
		return
	}
	if !lockAcquired {
		mci.Logger.Logf(slogger.INFO, "Cannot proceed with monitor because"+
			" the global lock could not be taken")
		return
	}

	// defer releasing the global lock
	defer func() {
		err := db.ReleaseGlobalLock(MonitorLockTitle)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error releasing global lock"+
				" from monitor - this is really bad: %v", err)
		}
	}()

	// log the start time
	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting monitor process at time %v",
		startTime)

	// run the monitoring processes
	if err := monitor.RunAllMonitoring(mciSettings); err != nil {
		mci.Logger.Logf(slogger.ERROR, "Error running monitoring: %v", err)
	}

	// log the runtime
	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.MonitorPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v",
			err)
	}

	mci.Logger.Logf(slogger.INFO, "Monitor took %v to run", runtime)
}
