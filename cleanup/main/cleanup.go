package main

import (
	"10gen.com/mci"
	"10gen.com/mci/cleanup"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

/********************************************************
Responsible for:
- checking on running tasks periodically
to see if they have timed out.
- checking on spawned hosts to see if they've
been idle for too long so we terminate.
- checking on hosts that have been spawned but
not yet marked as provisioned after a threshold
so we terminate.
- checking that hosts spawned for a distro
abide by the maxhosts field for that distro
********************************************************/

const (
	CleanupLockTitle = "cleanup"
)

func main() {
	mciSettings := mci.MustConfig()
	if mciSettings.Cleanup.LogFile != "" {
		mci.SetLogger(mciSettings.Cleanup.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	// take the global lock before doing anything
	lockAcquired, err := db.WaitTillAcquireGlobalLock(CleanupLockTitle, db.LockTimeout)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
		return
	}
	if !lockAcquired {
		mci.Logger.Logf(slogger.INFO, "Cannot proceed with cleanup because the global lock could not be taken")
		return
	}

	// defer releasing the global lock
	defer func() {
		err := db.ReleaseGlobalLock(CleanupLockTitle)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error releasing global lock from cleanup - this is really bad: %v", err)
		}
	}()

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting cleanup at time %v", startTime)
	mci.Logger.Logf(slogger.INFO, "Cleanup starting with db %v and config dir %v", mciSettings.Db, mciSettings.ConfigDir)

	// The order in which each of these commands is run is
	// important in several cases. e.g. We need to run
	// TerminateDecommissionedHosts _before_ we run
	// CheckTimeouts since the latter could change the
	// status of one or more already decommissioned hosts and
	// thus cause the former not to have any effect if the
	// latter is run before it.
	cleanup.TerminateDecommissionedHosts(mciSettings)
	cleanup.CheckTimeouts(mciSettings)
	cleanup.TerminateIdleHosts(mciSettings)
	cleanup.TerminateExcessHosts(mciSettings)
	cleanup.TerminateUnproductiveHosts(mciSettings)
	cleanup.WarnSlowProvisioningHosts(mciSettings)
	cleanup.TerminateUnprovisionedHosts(mciSettings)
	//cleanup.TerminateHungTasks(mciSettings)
	cleanup.TerminateStaleHosts(mciSettings)

	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.CleanupPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}

	mci.Logger.Logf(slogger.INFO, "Cleanup took %v to run", runtime)
}
