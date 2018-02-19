package monitor

import (
	"context"
	"sync"
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
	ReachabilityCheckInterval = 10 * time.Minute
	NumReachabilityWorkers    = 10
)

// responsible for monitoring and checking in on hosts
type hostMonitoringFunc func(context.Context, *evergreen.Settings) []error

// monitorReachability is a hostMonitoringFunc responsible for seeing if
// hosts are reachable or not. returns a slice of any errors that occur
func monitorReachability(ctx context.Context, settings *evergreen.Settings) []error {
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
			select {
			case h := <-hostsChan:
				if err := checkHostReachability(ctx, h, settings); err != nil {
					errChan <- errors.WithStack(err)
				}
			case <-ctx.Done():
				return
			}
		}()
	}

	errDone := make(chan struct{})
	go func() {
		defer close(errDone)
		select {
		case err := <-errChan:
			if err != nil {
				errs = append(errs, errors.Wrap(err, "error checking reachability"))
			}
		case <-ctx.Done():
			return
		}
	}()

	// check all of the hosts. continue on error so that other hosts can be
	// checked successfully
	for _, host := range hosts {
		if ctx.Err() != nil {
			close(hostsChan)
			return append(errs, errors.New("host checks aborted"))
		}
		hostsChan <- host
	}
	close(hostsChan)
	wg.Wait()
	close(errChan)

	<-errDone
	if ctx.Err() != nil {
		return append(errs, errors.New("host checks aborted"))
	}

	return errs
}

// check reachability for a single host, and take any necessary action
func checkHostReachability(ctx context.Context, host host.Host, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"runner":    RunnerName,
		"operation": "monitorReachability",
		"message":   "Running reachability check for host",
		"host":      host.Id,
	})

	// get a cloud version of the host
	cloudHost, err := cloud.GetCloudHost(ctx, &host, settings)
	if err != nil {
		return errors.Wrapf(err, "error getting cloud host for host %v: %v", host.Id)
	}

	// get the cloud status for the host
	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		return errors.Wrapf(err, "error getting cloud status for host %s", host.Id)
	}

	// take different action, depending on how the cloud provider reports the host's status
	switch cloudStatus {
	case cloud.StatusRunning:
		reachable := true

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

		// mark the host appropriately; this is a noop if the host status hasn't changed.
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
		if err := host.SetTerminated("external"); err != nil {
			return errors.Wrapf(err, "error setting host %s terminated", host.Id)
		}
	}

	// success
	return nil

}
