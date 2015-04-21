package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/model/host"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"strconv"
	"time"
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
type notificationBuilder func(*mci.MCISettings) ([]notification, error)

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
func spawnHostExpirationWarnings(mciSettings *mci.MCISettings) ([]notification,
	error) {

	mci.Logger.Logf(slogger.INFO, "Building spawned host expiration"+
		" warnings...")

	// sanity check, since the thresholds are supplied in code
	if len(spawnWarningThresholds) == 0 {
		mci.Logger.Logf(slogger.WARN, "there are no currently set warning"+
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

		mci.Logger.Logf(slogger.INFO, "Warning needs to be sent for threshold"+
			" '%v' for host %v", thresholdKey, h.Id)

		// we need to send a notification for the threshold for this host
		hostNotification := notification{
			recipient: h.StartedBy,
			subject:   fmt.Sprintf("%v host termination reminder", h.Distro.Id),
			message: fmt.Sprintf("Your %v host with id %v will be terminated"+
				" at %v, in %v minutes. Visit %v to extend its lifetime.",
				h.Distro.Id, h.Id,
				h.ExpirationTime.Format(time.RFC850),
				h.ExpirationTime.Sub(time.Now()),
				mciSettings.Ui.Url+"/ui/spawn"),
			threshold: thresholdKey,
			host:      h,
			callback: func(h host.Host, thresholdKey string) error {
				return h.SetExpirationNotification(thresholdKey)
			},
		}

		// add it to the list
		warnings = append(warnings, hostNotification)

	}

	mci.Logger.Logf(slogger.INFO, "Built %v warnings about imminently"+
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
func slowProvisioningWarnings(mciSettings *mci.MCISettings) ([]notification,
	error) {

	mci.Logger.Logf(slogger.INFO, "Building warnings for hosts taking a long"+
		" time to provision...")

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

		mci.Logger.Logf(slogger.INFO, "Slow-provisioning warning needs to"+
			" be sent for host %v", h.Id)

		// build the notification
		hostNotification := notification{
			recipient: mciSettings.Notify.SMTP.AdminEmail[0],
			subject: fmt.Sprintf("Host %v taking a long time to provision",
				h.Id),
			message: fmt.Sprintf("See %v/ui/host/%v",
				mciSettings.Ui.Url, h.Id),
			threshold: slowProvisioningWarning,
			host:      h,
			callback: func(h host.Host, s string) error {
				return h.SetExpirationNotification(s)
			},
		}

		// add it to the final list
		warnings = append(warnings, hostNotification)

	}

	mci.Logger.Logf(slogger.INFO, "Built %v warnings about hosts taking a"+
		" long time to provision", len(warnings))

	return warnings, nil
}
