package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/hostinit"
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

func main() {

	// read in settings, set log file appropriately
	mciSettings := mci.MustConfig()
	if mciSettings.HostInit.LogFile != "" {
		mci.SetLogger(mciSettings.HostInit.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	// defer checking for a panic in the execution
	defer func() {
		if r := recover(); r != nil {
			mci.Logger.Errorf(slogger.ERROR, "Hostinit execution panicked: %v", r)
		}
	}()

	// track start time
	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting hostinit at time %v", startTime)

	// create an instance of HostInit to do the initialization
	init := &hostinit.HostInit{
		MCISettings: mciSettings,
	}

	// kick off setup on all necessary hosts
	if err := init.SetupReadyHosts(); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error thrown in hostinit: %v", err)
	}

	// track total run time
	// TODO: this may not be necessary, since it will be at least the time that the
	//  longest setup script took to run
	runtime := time.Now().Sub(startTime)
	if err := model.SetProcessRuntimeCompleted(mci.HostInitPackage, runtime); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Hostinit took %v to run", runtime)

}
