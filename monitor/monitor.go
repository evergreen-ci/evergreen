package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
)

var (
	// the functions the task monitor will run through to find tasks needing
	// to be cleaned up
	defaultTaskFlaggingFuncs = []taskFlaggingFunc{
		flagTimedOutHeartbeats,
	}

	// the functions the host monitor will run through to find hosts needing
	// to be terminated
	defaultHostFlaggingFuncs = []hostFlaggingFunc{
	/*
		flagDecommissionedHosts,
		flagIdleHosts,
		flagExcessHosts,
		flagUnprovisionedHosts,
		flagProvisioningFailedHosts,
		flagExpiredHosts,
	*/
	}

	// the functions the host monitor will run through to do simpler checks
	defaultHostMonitoringFuncs = []hostMonitoringFunc{
	//monitorReachability,
	}

	// the functions the notifier will use to build notifications that need
	// to be sent
	defaultNotificationBuilders = []notificationBuilder{
	//spawnHostExpirationWarnings,
	//slowProvisioningWarnings,
	}
)

// run all monitoring functions
func RunAllMonitoring(mciSettings *mci.MCISettings) error {

	// load in all of the distros
	distros, err := model.LoadDistros(mciSettings.ConfigDir)
	if err != nil {
		return fmt.Errorf("error loading in distros from config dir %v: %v",
			mciSettings.ConfigDir, err)
	}

	// fetch the project refs, which we will use to get all of the projects
	projectRefs, err := model.FindAllProjectRefs()
	if err != nil {
		return fmt.Errorf("error loading in project refs: %v", err)
	}

	// turn the project refs into a map of the project id -> project
	projects := map[string]model.Project{}
	for _, ref := range projectRefs {
		project, err := model.FindProject("", ref.Identifier,
			mciSettings.ConfigDir)

		// continue on error to stop the whole monitoring process from
		// being held up
		if err != nil {
			mci.Logger.Logf(slogger.ERROR, "error finding project %v: %v",
				ref.Identifier, err)
			continue
		}

		projects[project.Identifier] = *project
	}

	// initialize the task monitor
	taskMonitor := &TaskMonitor{
		flaggingFuncs: defaultTaskFlaggingFuncs,
	}

	// clean up any necessary tasks
	errs := taskMonitor.CleanupTasks(projects)
	for _, err := range errs {
		mci.Logger.Logf(slogger.ERROR, "Error cleaning up tasks: %v", err)
	}

	// initialize the host monitor
	hostMonitor := &HostMonitor{
		flaggingFuncs:   defaultHostFlaggingFuncs,
		monitoringFuncs: defaultHostMonitoringFuncs,
	}

	// clean up any necessary hosts
	errs = hostMonitor.CleanupHosts(distros, mciSettings)
	for _, err := range errs {
		mci.Logger.Logf(slogger.ERROR, "Error cleaning up hosts: %v", err)
	}

	// run monitoring checks
	errs = hostMonitor.RunMonitoringChecks(mciSettings)
	for _, err := range errs {
		mci.Logger.Logf(slogger.ERROR, "Error running host monitoring"+
			" checks: %v", err)
	}

	// initialize the notifier
	notifier := &Notifier{
		notificationBuilders: defaultNotificationBuilders,
	}

	// send notifications
	errs = notifier.Notify(mciSettings)
	for _, err := range errs {
		mci.Logger.Logf(slogger.ERROR, "Error sending notifications: %v", err)
	}

	return nil

}
