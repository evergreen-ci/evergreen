package monitor

import (
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// how long to wait in between reachability checks
	ReachabilityCheckInterval = 10 * time.Minute
	NumReachabilityWorkers    = 100
)

// responsible for monitoring and checking in on hosts
type hostMonitoringFunc func(*evergreen.Settings) []error

// monitorReachability is a hostMonitoringFunc responsible for seeing if
// hosts are reachable or not. returns a slice of any errors that occur
func monitorReachability(settings *evergreen.Settings) []error {
	grip.Info(message.Fields{
		"runner":    RunnerName,
		"operation": "monitorReachability",
		"message":   "Running reachability checks",
	})

	// used to store any errors that occur
	var errs []error

	// fetch all hosts that have not been checked recently
	// (> 10 minutes ago)
	threshold := time.Now().Add(-ReachabilityCheckInterval)
	hosts, err := host.Find(host.ByNotMonitoredSince(threshold))
	if err != nil {
		errs = append(errs, errors.Wrap(err, "error finding hosts not monitored recently"))
		return errs
	}

	workers := NumReachabilityWorkers
	if len(hosts) < workers {
		workers = len(hosts)
	}

	wg := sync.WaitGroup{}

	wg.Add(workers)

	hostsChan := make(chan host.Host, workers)
	errChan := make(chan error, workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for host := range hostsChan {
				if err := checkHostReachability(host, settings); err != nil {
					errChan <- errors.WithStack(err)
				}
			}
		}()
	}

	errDone := make(chan struct{})
	go func() {
		defer close(errDone)
		for err := range errChan {
			errs = append(errs, errors.Wrap(err, "error checking reachability"))
		}
	}()

	// check all of the hosts. continue on error so that other hosts can be
	// checked successfully
	for _, host := range hosts {
		hostsChan <- host
	}
	close(hostsChan)
	wg.Wait()
	close(errChan)

	<-errDone
	return errs
}

// check reachability for a single host, and take any necessary action
func checkHostReachability(host host.Host, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"runner":    RunnerName,
		"operation": "monitorReachability",
		"message":   "Running reachability check for host",
		"host":      host.Id,
	})

	// get a cloud version of the host
	cloudHost, err := providers.GetCloudHost(&host, settings)
	if err != nil {
		return errors.Wrapf(err, "error getting cloud host for host %v: %v", host.Id)
	}

	// get the cloud status for the host
	cloudStatus, err := cloudHost.GetInstanceStatus()
	if err != nil {
		return errors.Wrapf(err, "error getting cloud status for host %s", host.Id)
	}

	// take different action, depending on how the cloud provider reports the host's status
	switch cloudStatus {
	case cloud.StatusRunning:
		// check if the host is reachable via SSH
		reachable, err := cloudHost.IsSSHReachable()
		if err != nil {
			return errors.Wrapf(err, "error checking ssh reachability for host %s", host.Id)
		}

		// log the status update if the reachability of the host is changing
		if host.Status == evergreen.HostUnreachable && reachable {
			grip.Info(message.Fields{
				"runner":    RunnerName,
				"operation": "monitorReachability",
				"message":   "Setting host as reachable",
				"host":      host.Id,
			})
		} else if host.Status != evergreen.HostUnreachable && !reachable {
			grip.Info(message.Fields{
				"runner":    RunnerName,
				"operation": "monitorReachability",
				"message":   "Setting host as unreachable",
				"host":      host.Id,
			})
		}

		// mark the host appropriately
		if err := host.UpdateReachability(reachable); err != nil {
			return errors.Wrapf(err, "error updating reachability for host %s", host.Id)
		}
	case cloud.StatusTerminated:
		grip.Info(message.Fields{
			"runner":    RunnerName,
			"operation": "monitorReachability",
			"message":   "host terminated externally",
			"host":      host.Id,
			"distro":    host.Distro.Id,
		})
		event.LogHostTerminatedExternally(host.Id)

		// the instance was terminated from outside our control
		if err := host.SetTerminated(); err != nil {
			return errors.Wrapf(err, "error setting host %s terminated", host.Id)
		}
	}

	// success
	return nil

}
