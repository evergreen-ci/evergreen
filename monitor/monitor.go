package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var (
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

	// initialize the host monitor
	hostMonitor := &HostMonitor{
		flaggingFuncs: defaultHostFlaggingFuncs,
	}

	// clean up any necessary hosts
	err = hostMonitor.CleanupHosts(ctx, distros, settings)
	grip.Error(message.WrapError(err, message.Fields{
		"runner":  RunnerName,
		"message": "Error cleaning up hosts",
	}))

	if ctx.Err() != nil {
		return errors.New("monitor canceled")
	}

	// initialize the notifier
	notifier := &Notifier{
		notificationBuilders: defaultNotificationBuilders,
	}

	// send notifications
	err = notifier.Notify(settings)
	grip.Error(message.WrapError(err, message.Fields{
		"runner":  RunnerName,
		"message": "Error sending notifications",
	}))

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
		if err = alerts.RunSpawnWarningTriggers(&h); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  RunnerName,
				"message": "Error queuing alert",
				"host":    h.Id,
			}))
		}
	}

	return nil
}
