package monitor

import (
	"fmt"
	"strconv"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
)

// the warning thresholds for spawned hosts, in decreasing order of recency
var (
	spawnWarningThresholds = []time.Duration{
		time.Duration(2) * time.Hour,
		time.Duration(12) * time.Hour,
	}

	// the threshold for what is considered "slow provisioning"
	slowProvisioningThreshold = 20 * time.Minute
)

const (
	// the key for a host's notifications about slow provisioning
	slowProvisioningWarning = "late-provision-warning"
)

// a function that outputs any necessary notifications
type notificationBuilder func(*evergreen.Settings) ([]notification, error)

// contains info about a notification that should be sent
type notification struct {
	recipient string
	subject   string
	message   string
	threshold string
	host      host.Host

	// to fire after the notification is sent - usually intended to set db
	// fields indicating that the notification has been sent
	callback func(host.Host, string) error
}

// spawnHostExpirationWarnings is a notificationBuilder to build any necessary
// warnings about hosts that will be expiring soon (but haven't expired yet)
func spawnHostExpirationWarnings(settings *evergreen.Settings) ([]notification,
	error) {

	evergreen.Logger.Logf(slogger.INFO, "Building spawned host expiration"+
		" warnings...")

	// sanity check, since the thresholds are supplied in code
	if len(spawnWarningThresholds) == 0 {
		evergreen.Logger.Logf(slogger.WARN, "there are no currently set warning"+
			" thresholds for spawned hosts - users will not receive emails"+
			" warning them of imminent host expiration")
		return nil, nil
	}

	// assumed to be the first warning threshold (the least recent one)
	firstWarningThreshold :=
		spawnWarningThresholds[len(spawnWarningThresholds)-1]

	// find all spawned hosts that have passed at least one warning threshold
	now := time.Now()
	thresholdTime := now.Add(firstWarningThreshold)
	hosts, err := host.Find(host.ByExpiringBetween(now, thresholdTime))
	if err != nil {
		return nil, fmt.Errorf("error finding spawned hosts that will be"+
			" expiring soon: %v", err)
	}

	// the eventual list of warning notifications to be sent
	warnings := []notification{}

	for _, h := range hosts {

		// figure out the most recent expiration notification threshold the host
		// has crossed
		threshold := lastWarningThresholdCrossed(&h)

		// for keying into the host's notifications map
		thresholdKey := strconv.Itoa(int(threshold.Minutes()))

		// if a notification has already been sent for the threshold for this
		// host, skip it
		if h.Notifications[thresholdKey] {
			continue
		}

		evergreen.Logger.Logf(slogger.INFO, "Warning needs to be sent for threshold"+
			" '%v' for host %v", thresholdKey, h.Id)

		// fetch information about the user we are notifying
		userToNotify, err := user.FindOne(user.ById(h.StartedBy))
		if err != nil {
			return nil, fmt.Errorf("error finding user to notify by Id %v: %v", h.StartedBy, err)

		}

		// if we didn't find a user (in the case of testing) set the timezone to ""
		// to avoid triggering a nil pointer exception
		timezone := ""
		if userToNotify != nil {
			timezone = userToNotify.Settings.Timezone
		}

		var expirationTimeFormatted string
		// use our fetched information to load proper time zone to notify the user with
		// (if time zone is empty, defaults to UTC)
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error loading timezone for email format with user_id %v: %v", userToNotify.Id, err)
			expirationTimeFormatted = h.ExpirationTime.Format(time.RFC1123)
		} else {
			expirationTimeFormatted = h.ExpirationTime.In(loc).Format(time.RFC1123)
		}
		// we need to send a notification for the threshold for this host
		hostNotification := notification{
			recipient: h.StartedBy,
			subject:   fmt.Sprintf("%v host termination reminder", h.Distro.Id),
			message: fmt.Sprintf("Your %v host with id %v will be terminated"+
				" at %v. Visit %v to extend its lifetime.",
				h.Distro.Id, h.Id,
				expirationTimeFormatted,
				settings.Ui.Url+"/ui/spawn"),
			threshold: thresholdKey,
			host:      h,
			callback: func(h host.Host, thresholdKey string) error {
				return h.SetExpirationNotification(thresholdKey)
			},
		}

		// add it to the list
		warnings = append(warnings, hostNotification)

	}

	evergreen.Logger.Logf(slogger.INFO, "Built %v warnings about imminently"+
		" expiring hosts", len(warnings))

	return warnings, nil
}

// determine the most recently crossed expiration notification threshold for
// the host. any host passed into this function is assumed to have crossed
// at least the least recent threshold
func lastWarningThresholdCrossed(host *host.Host) time.Duration {

	// how long til the host expires
	tilExpiration := host.ExpirationTime.Sub(time.Now())

	// iterate through the thresholds - since they are kept in sorted order,
	// the first one crossed will be the most recent one crossed
	for _, threshold := range spawnWarningThresholds {
		if tilExpiration <= threshold {
			return threshold
		}
	}

	// should never be reached
	return time.Duration(0)

}

// slowProvisioningWarnings is a notificationBuilder to build any necessary
// warnings about hosts that are taking a long time to provision
func slowProvisioningWarnings(settings *evergreen.Settings) ([]notification,
	error) {

	evergreen.Logger.Logf(slogger.INFO, "Building warnings for hosts taking a long"+
		" time to provision...")

	if settings.Notify.SMTP == nil {
		return []notification{}, fmt.Errorf("no notification emails configured")
	}

	// fetch all hosts that are taking too long to provision
	threshold := time.Now().Add(-slowProvisioningThreshold)
	hosts, err := host.Find(host.ByUnprovisionedSince(threshold))
	if err != nil {
		return nil, fmt.Errorf("error finding unprovisioned hosts: %v", err)
	}

	// the list of warning notifications that will be returned
	warnings := []notification{}

	for _, h := range hosts {

		// if a warning has been sent for the host, skip it
		if h.Notifications[slowProvisioningWarning] {
			continue
		}

		evergreen.Logger.Logf(slogger.INFO, "Slow-provisioning warning needs to"+
			" be sent for host %v", h.Id)

		// build the notification
		hostNotification := notification{
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

	evergreen.Logger.Logf(slogger.INFO, "Built %v warnings about hosts taking a"+
		" long time to provision", len(warnings))

	return warnings, nil
}
