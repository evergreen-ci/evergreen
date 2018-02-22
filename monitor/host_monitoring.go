package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// how long to wait in between reachability checks
	reachabilityCheckInterval = 10 * time.Minute
)

// responsible for monitoring and checking in on hosts
type hostMonitoringFunc func(context.Context, *evergreen.Settings) error

// monitorReachability is a hostMonitoringFunc responsible for seeing if
// hosts are reachable or not. returns a slice of any errors that occur
func monitorReachability(ctx context.Context, settings *evergreen.Settings) error {
	// fetch all hosts that have not been checked recently
	// (> 10 minutes ago)
	startAt := time.Now()
	threshold := time.Now().Add(-reachabilityCheckInterval)
	hosts, err := host.Find(host.ByNotMonitoredSince(threshold))
	if err != nil {
		return errors.Wrap(err, "error finding hosts not monitored recently")
	}

	catcher := grip.NewBasicCatcher()

checkLoop:
	for _, h := range hosts {
		// get a cloud version of the host
		cloudHost, err := cloud.GetCloudHost(ctx, &h, settings)
		if err != nil {
			catcher.Add(errors.Wrapf(err, "error getting cloud host for host %v: %v", h.Id))
			continue checkLoop
		}

		// get the cloud status for the host
		cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
		if err != nil {
			catcher.Add(errors.Wrapf(err, "error getting cloud status for host %s", h.Id))
			continue checkLoop
		}

		// take different action, depending on how the cloud provider reports the host's status
		switch cloudStatus {
		case cloud.StatusRunning:
			// mark the host appropriately; this is a noop if the host status hasn't changed.
			if err := h.UpdateReachability(true); err != nil {
				catcher.Add(errors.Wrapf(err, "error updating reachability for host %s", h.Id))
				continue checkLoop
			}

		case cloud.StatusTerminated:
			grip.Info(message.Fields{
				"runner":    RunnerName,
				"operation": "monitorReachability",
				"message":   "host terminated externally",
				"host":      h.Id,
				"distro":    h.Distro.Id,
			})
			event.LogHostTerminatedExternally(h.Id)

			// the instance was terminated from outside our control
			if err := host.SetTerminated("external"); err != nil {
				catcher.Add(errors.Wrapf(err, "error setting host %s terminated", h.Id))
				continue checkLoop
			}
		}
	}

	grip.Info(message.Fields{
		"runner":       RunnerName,
		"operation":    "legacy-reachability-stat",
		"hosts":        len(hosts),
		"errors":       catcher.Len(),
		"runtime_secs": time.Since(startAt).Seconds(),
	})

	return catcher.Resovle()
}
