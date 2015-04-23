package main

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/notify"
	"github.com/10gen-labs/slogger/v1"
	"time"
)

func main() {
	mciSettings := mci.MustConfig()
	if mciSettings.Notify.LogFile != "" {
		mci.SetLogger(mciSettings.Notify.LogFile)
	}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mciSettings))

	startTime := time.Now()
	mci.Logger.Logf(slogger.INFO, "Starting notifications at time %v", startTime)
	mci.Logger.Logf(slogger.INFO, "Running notifications with db %v and notifications configuration %v/%v", mciSettings.Db, mciSettings.ConfigDir, mci.NotificationsFile)

	err := notify.RunPipeline(mciSettings)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error running notifications pipeline: %v", err)
		return
	}

	runtime := time.Now().Sub(startTime)
	err = model.SetProcessRuntimeCompleted(mci.NotifyPackage, runtime)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating process status: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Notifications took %v to run", runtime)
}
