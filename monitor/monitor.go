package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var (
	// the functions the notifier will use to build notifications that need
	// to be sent
	defaultNotificationBuilders = []notificationBuilder{
		spawnHostExpirationWarnings,
		slowProvisioningWarnings,
	}
)

// run all monitoring functions
func RunAllMonitoring(ctx context.Context, settings *evergreen.Settings) error {
	// initialize the notifier
	notifier := &Notifier{
		notificationBuilders: defaultNotificationBuilders,
	}

	// send notifications
	err := notifier.Notify(settings)
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
