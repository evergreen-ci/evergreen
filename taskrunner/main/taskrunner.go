package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/taskrunner"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

const (
	TaskRunnerLockTitle = "taskrunner"
)

// Main method for the task runner.
func main() {

	// load in and set appropriate configuration
	mciSettings := mci.MustConfig()
	if mciSettings.TaskRunner.LogFile != "" {
		mci.SetLogger(mciSettings.TaskRunner.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	// take the global lock before doing anything
	lockAcquired, err := db.WaitTillAcquireGlobalLock(TaskRunnerLockTitle,
		db.LockTimeout)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
		return
	}
	if !lockAcquired {
		mci.Logger.Logf(slogger.WARN, "Cannot proceed with taskrunner because"+
			" the global lock could not be taken")
		return
	}

	// defer releasing the global lock
	defer func() {
		err := db.ReleaseGlobalLock(TaskRunnerLockTitle)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error releasing global lock"+
				" from taskrunner - this is really bad: %v", err)
		}
	}()

	taskRunnerInstance := taskrunner.NewTaskRunner(mciSettings)
	startTime := time.Now()

	if err = taskRunnerInstance.RunTasks(); err != nil {
		panic(fmt.Sprintf("error running task runner: %v", err))
	}

	// report the status
	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.TaskRunnerPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v",
			err)
	}
}
