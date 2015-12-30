package monitor

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/host"
	"time"
)

const (
	// how long to wait in between reachability checks
	ReachabilityCheckInterval = 10 * time.Minute
)

// responsible for monitoring and checking in on hosts
type hostMonitoringFunc func(*evergreen.Settings) []error

// monitorReachability is a hostMonitoringFunc responsible for seeing if
// hosts are reachable or not. returns a slice of any errors that occur
func monitorReachability(settings *evergreen.Settings) []error {

	evergreen.Logger.Logf(slogger.INFO, "Running reachability checks...")

	// used to store any errors that occur
	var errors []error

	// fetch all hosts that have not been checked recently
	// (> 10 minutes ago)
	threshold := time.Now().Add(-ReachabilityCheckInterval)
	hosts, err := host.Find(host.ByNotMonitoredSince(threshold))
	if err != nil {
		errors = append(errors, fmt.Errorf("error finding hosts not monitored recently: %v", err))
		return errors
	}

	// check all of the hosts. continue on error so that other hosts can be
	// checked successfully
	for _, host := range hosts {
		if err := checkHostReachability(host, settings); err != nil {
			errors = append(errors, fmt.Errorf("error checking reachability for host %v: %v", host.Id, err))
			continue
		}

	}

	evergreen.Logger.Logf(slogger.INFO, "Finished running host reachability checks")

	return errors
}

// check reachability for a single host, and take any necessary action
func checkHostReachability(host host.Host, settings *evergreen.Settings) error {
	evergreen.Logger.Logf(slogger.INFO, "Running reachability check for host %v...", host.Id)

	// get a cloud version of the host
	cloudHost, err := providers.GetCloudHost(&host, settings)
	if err != nil {
		return fmt.Errorf("error getting cloud host for host %v: %v", host.Id, err)
	}

	// get the cloud status for the host
	cloudStatus, err := cloudHost.GetInstanceStatus()
	if err != nil {
		return fmt.Errorf("error getting cloud status for host %v: %v", host.Id, err)
	}

	// take different action, depending on how the cloud provider reports the host's status
	switch cloudStatus {
	case cloud.StatusRunning:
		// check if the host is reachable via SSH
		reachable, err := cloudHost.IsSSHReachable()
		if err != nil {
			return fmt.Errorf("error checking ssh reachability for host %v: %v", host.Id, err)
		}

		// log the status update if the reachability of the host is changing
		if host.Status == evergreen.HostUnreachable && reachable {
			evergreen.Logger.Logf(slogger.INFO, "Setting host %v as reachable", host.Id)
		} else if host.Status != evergreen.HostUnreachable && !reachable {
			evergreen.Logger.Logf(slogger.INFO, "Setting host %v as unreachable", host.Id)
		}

		// mark the host appropriately
		if err := host.UpdateReachability(reachable); err != nil {
			return fmt.Errorf("error updating reachability for host %v: %v", host.Id, err)
		}
	case cloud.StatusTerminated:
		evergreen.Logger.Logf(slogger.INFO, "Host %v terminated externally; updating db status to terminated", host.Id)

		// the instance was terminated from outside our control
		if err := host.SetTerminated(); err != nil {
			return fmt.Errorf("error setting host %v terminated: %v", host.Id, err)
		}
	}

	// success
	return nil

}
