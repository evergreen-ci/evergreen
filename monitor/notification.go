package monitor

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

// the warning thresholds for spawned hosts, in decreasing order of recency
var (
	// the threshold for what is considered "slow provisioning"
	slowProvisioningThreshold = 20 * time.Minute
)

const (
	// the key for a host's notifications about slow provisioning
	slowProvisioningWarning = "late-provision-warning"
)

// a function that outputs any necessary notifications
type NotificationBuilder func(*evergreen.Settings) ([]Notification, error)

// contains info about a notification that should be sent
type Notification struct {
	recipient string
	subject   string
	message   string
	threshold string
	host      host.Host

	// to fire after the notification is sent - usually intended to set db
	// fields indicating that the notification has been sent
	callback func(host.Host, string) error
}

// slowProvisioningWarnings is a notificationBuilder to build any necessary
// warnings about hosts that are taking a long time to provision
func SlowProvisioningWarnings(settings *evergreen.Settings) ([]Notification, error) {
	if len(settings.Notify.SMTP.AdminEmail) == 0 {
		return []Notification{}, errors.New("no notification emails configured")
	}

	// fetch all hosts that are taking too long to provision
	threshold := time.Now().Add(-slowProvisioningThreshold)
	hosts, err := host.Find(host.ByUnprovisionedSince(threshold))
	if err != nil {
		return nil, errors.Wrap(err, "error finding unprovisioned hosts")
	}

	// the list of warning notifications that will be returned
	warnings := []Notification{}

	for _, h := range hosts {

		// if a warning has been sent for the host, skip it
		if h.Notifications[slowProvisioningWarning] {
			continue
		}

		// build the notification
		hostNotification := Notification{
			recipient: settings.Notify.SMTP.AdminEmail[0],
			subject: fmt.Sprintf("Host %v taking a long time to provision",
				h.Id),
			message: fmt.Sprintf("See %v/ui/host/%v",
				settings.Ui.Url, h.Id),
			threshold: slowProvisioningWarning,
			host:      h,
			callback: func(h host.Host, s string) error {
				return h.SetExpirationNotification(s)
			},
		}

		// add it to the final list
		warnings = append(warnings, hostNotification)
	}

	return warnings, nil
}
