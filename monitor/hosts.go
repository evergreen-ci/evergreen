package monitor

import (
	"fmt"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/util"
)

// responsible for running regular monitoring of hosts
type HostMonitor struct {
	// will be used to determine what hosts need to be terminated
	flaggingFuncs []hostFlagger

	// will be used to perform regular checks on hosts
	monitoringFuncs []hostMonitoringFunc
}

// run through the list of host monitoring functions. returns any errors that
// occur while running the monitoring functions
func (hm *HostMonitor) RunMonitoringChecks(settings *evergreen.Settings) []error {

	evergreen.Logger.Logf(slogger.INFO, "Running host monitoring checks...")

	// used to store any errors that occur
	var errors []error

	for _, f := range hm.monitoringFuncs {

		// continue on error to allow the other monitoring functions to run
		if errs := f(settings); errs != nil {
			for _, err := range errs {
				errors = append(errors, err)
			}
		}

	}

	evergreen.Logger.Logf(slogger.INFO, "Finished running host monitoring checks")

	return errors

}

// run through the list of host flagging functions, finding all hosts that
// need to be terminated and terminating them
func (hm *HostMonitor) CleanupHosts(distros []distro.Distro, settings *evergreen.Settings) []error {

	evergreen.Logger.Logf(slogger.INFO, "Running host cleanup...")

	// used to store any errors that occur
	var errors []error

	for idx, flagger := range hm.flaggingFuncs {
		evergreen.Logger.Logf(slogger.INFO, "Searching for flagged hosts under criteria: %v", flagger.Reason)
		// find the next batch of hosts to terminate
		hostsToTerminate, err := flagger.hostFlaggingFunc(distros, settings)
		evergreen.Logger.Logf(slogger.INFO, "Found %v hosts flagged for '%v'", len(hostsToTerminate), flagger.Reason)

		// continuing on error so that one wonky flagging function doesn't
		// stop others from running
		if err != nil {
			errors = append(errors, fmt.Errorf("error flagging hosts to be terminated: %v", err))
			continue
		}

		evergreen.Logger.Logf(slogger.INFO, "Check %v: found %v hosts to be terminated", idx, len(hostsToTerminate))

		// terminate all of the dead hosts. continue on error to allow further
		// termination to work
		if errs := terminateHosts(hostsToTerminate, settings, flagger.Reason); errs != nil {
			for _, err := range errs {
				errors = append(errors, fmt.Errorf("error terminating host: %v", err))
			}
			continue
		}

	}

	return errors

}

// terminate the passed-in slice of hosts. returns any errors that occur
// terminating the hosts
func terminateHosts(hosts []host.Host, settings *evergreen.Settings, reason string) []error {
	errChan := make(chan error)
	for _, h := range hosts {
		evergreen.Logger.Logf(slogger.INFO, "Terminating host %v", h.Id)
		// terminate the host in a goroutine, passing the host in as a parameter
		// so that the variable isn't reused for subsequent iterations
		go func(hostToTerminate host.Host) {
			errChan <- func() error {
				event.LogMonitorOperation(hostToTerminate.Id, reason)
				err := util.RunFunctionWithTimeout(func() error {
					return terminateHost(&hostToTerminate, settings)
				}, 12*time.Minute)
				if err != nil {
					if err == util.ErrTimedOut {
						return fmt.Errorf("timeout terminating host %v", hostToTerminate.Id)
					}
					return fmt.Errorf("error terminating host %v: %v", hostToTerminate.Id, err)
				}
				evergreen.Logger.Logf(slogger.INFO, "Successfully terminated host %v", hostToTerminate.Id)
				return nil
			}()
		}(h)
	}
	var errors []error
	for range hosts {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// helper to terminate a single host
func terminateHost(host *host.Host, settings *evergreen.Settings) error {

	// convert the host to a cloud host
	cloudHost, err := providers.GetCloudHost(host, settings)
	if err != nil {
		return fmt.Errorf("error getting cloud host for %v: %v", host.Id, err)
	}

	// run teardown script if we have one, sending notifications if things go awry
	if host.Distro.Teardown != "" && host.Provisioned {
		evergreen.Logger.Logf(slogger.ERROR, "Running teardown script for host %v", host.Id)
		if err := runHostTeardown(host, cloudHost); err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error running teardown script for %v: %v", host.Id, err)
			subj := fmt.Sprintf("%v Error running teardown for host %v",
				notify.TeardownFailurePreface, host.Id)
			if err := notify.NotifyAdmins(subj, err.Error(), settings); err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
			}
		}
	}

	// terminate the instance
	if err := cloudHost.TerminateInstance(); err != nil {
		return fmt.Errorf("error terminating host %v: %v", host.Id, err)
	}

	return nil
}

func runHostTeardown(h *host.Host, cloudHost *cloud.CloudHost) error {
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return fmt.Errorf("error getting ssh options for host %v: %v", h.Id, err)
	}
	startTime := time.Now()
	logs, err := hostutil.RunRemoteScript(h, "teardown.sh", sshOptions)
	event.LogHostTeardown(h.Id, logs, err == nil, time.Since(startTime))
	if err != nil {
		return fmt.Errorf("error (%v) running teardown.sh over ssh: %v", err, logs)
	}
	return nil
}
