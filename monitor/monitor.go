package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

var (
	// the functions the task monitor will run through to find tasks needing
	// to be cleaned up
	defaultTaskFlaggingFuncs = []taskFlaggingFunc{
		flagTimedOutHeartbeats,
	}

	// the functions the host monitor will run through to find hosts needing
	// to be terminated
	defaultHostFlaggingFuncs = []hostFlagger{
		{flagDecommissionedHosts, "decommissioned"},
		{flagUnreachableHosts, "unreachable"},
		{flagIdleHosts, "idle"},
		{flagExcessHosts, "excess"},
		{flagUnprovisionedHosts, "provision_timeout"},
		{flagProvisioningFailedHosts, "provision_failed"},
		{flagExpiredHosts, "expired"},
	}

	// the functions the host monitor will run through to do simpler checks
	defaultHostMonitoringFuncs = []hostMonitoringFunc{
		monitorReachability,
	}

	// the functions the notifier will use to build notifications that need
	// to be sent
	defaultNotificationBuilders = []notificationBuilder{
		spawnHostExpirationWarnings,
		slowProvisioningWarnings,
	}
)

// run all monitoring functions
func RunAllMonitoring(ctx context.Context, settings *evergreen.Settings) error {

	// load in all of the distros
	distros, err := distro.Find(db.Q{})
	if err != nil {
		return errors.Wrap(err, "error finding distros")
	}

	// fetch the project refs, which we will use to get all of the projects
	projectRefs, err := model.FindAllProjectRefs()
	if err != nil {
		return errors.Wrap(err, "error loading in project refs")
	}

	// turn the project refs into a map of the project id -> project
	projects := map[string]model.Project{}
	var project *model.Project

	for _, ref := range projectRefs {
		// only monitor projects that are enabled
		if !ref.Enabled {
			continue
		}

		if ctx.Err() != nil {
			return errors.New("monitor canceled")
		}

		project, err = model.FindProject("", &ref)

		// continue on error to stop the whole monitoring process from
		// being held up
		if err != nil {
			grip.Errorf("error finding project %s: %+v", ref.Identifier, err)
			continue
		}

		if project == nil {
			grip.Errorf("no project entry found for ref %s", ref.Identifier)
			continue
		}

		projects[project.Identifier] = *project
	}

	// initialize the task monitor
	taskMonitor := &TaskMonitor{
		flaggingFuncs: defaultTaskFlaggingFuncs,
	}

	// clean up any necessary tasks
	for _, err := range taskMonitor.CleanupTasks(ctx, projects) {
		grip.Error(errors.Wrap(err, "Error cleaning up tasks"))
	}

	if ctx.Err() != nil {
		return errors.New("monitor canceled")
	}

	// initialize the host monitor
	hostMonitor := &HostMonitor{
		flaggingFuncs:   defaultHostFlaggingFuncs,
		monitoringFuncs: defaultHostMonitoringFuncs,
	}
	// clean up any necessary hosts
	for _, err := range hostMonitor.CleanupHosts(ctx, distros, settings) {
		grip.Error(errors.Wrap(err, "Error cleaning up hosts"))
	}

	if ctx.Err() != nil {
		return errors.New("monitor canceled")
	}

	// run monitoring checks
	for _, err := range hostMonitor.RunMonitoringChecks(ctx, settings) {
		grip.Error(errors.Wrap(err, "Error running host monitoring checks"))
	}

	if ctx.Err() != nil {
		return errors.New("monitor canceled")
	}

	// initialize the notifier
	notifier := &Notifier{
		notificationBuilders: defaultNotificationBuilders,
	}

	// send notifications
	for _, err := range notifier.Notify(settings) {
		grip.Error(errors.Wrap(err, "Error sending notifications"))
	}

	// Do alerts for spawnhosts - collect all hosts expiring in the next 12 hours.
	// The trigger logic will filter out any hosts that aren't in a notification window, or have
	// already have alerts sent.
	now := time.Now()
	thresholdTime := now.Add(12 * time.Hour)
	expiringSoonHosts, err := host.Find(host.ByExpiringBetween(now, thresholdTime))
	if err != nil {
		return errors.WithStack(err)
	}

	for _, h := range expiringSoonHosts {
		err := alerts.RunSpawnWarningTriggers(&h)

		grip.Error(errors.Wrap(err, "Error queuing alert"))
	}

	return nil
}
