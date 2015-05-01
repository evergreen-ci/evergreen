package monitor

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"sync"
	"time"
)

// responsible for running regular monitoring of hosts
type HostMonitor struct {
	// will be used to determine what hosts need to be terminated
	flaggingFuncs []hostFlaggingFunc

	// will be used to perform regular checks on hosts
	monitoringFuncs []hostMonitoringFunc
}

// run through the list of host monitoring functions. returns any errors that
// occur while running the monitoring functions
func (self *HostMonitor) RunMonitoringChecks(mciSettings *evergreen.MCISettings) []error {

	evergreen.Logger.Logf(slogger.INFO, "Running host monitoring checks...")

	// used to store any errors that occur
	var errors []error

	for _, f := range self.monitoringFuncs {

		// continue on error to allow the other monitoring functions to run
		if errs := f(mciSettings); errs != nil {
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
func (self *HostMonitor) CleanupHosts(distros []distro.Distro, settings *evergreen.MCISettings) []error {

	evergreen.Logger.Logf(slogger.INFO, "Running host cleanup...")

	// used to store any errors that occur
	var errors []error

	for idx, f := range self.flaggingFuncs {
		// find the next batch of hosts to terminate
		hostsToTerminate, err := f(distros, settings)

		// continuing on error so that one wonky flagging function doesn't
		// stop others from running
		if err != nil {
			errors = append(errors, fmt.Errorf("error flagging hosts to"+
				" be terminated: %v", err))
			continue
		}

		evergreen.Logger.Logf(slogger.INFO, "Check %v: found %v hosts to be"+
			" terminated", idx, len(hostsToTerminate))

		// terminate all of the dead hosts. continue on error to allow further
		// termination to work
		if errs := terminateHosts(hostsToTerminate, settings); errs != nil {
			for _, err := range errs {
				errors = append(errors, fmt.Errorf("error terminating host:"+
					" %v", err))
			}
			continue
		}

	}

	return errors

}

// terminate the passed-in slice of hosts. returns any errors that occur
// terminating the hosts
func terminateHosts(hosts []host.Host, mciSettings *evergreen.MCISettings) []error {

	// used to store any errors that occur
	var errors []error

	// for terminating the different hosts in parallel
	waitGroup := &sync.WaitGroup{}

	// to ensure thread-safe appending to the errors
	errsLock := &sync.Mutex{}

	for _, h := range hosts {

		evergreen.Logger.Logf(slogger.INFO, "Terminating host %v...", h.Id)

		waitGroup.Add(1)

		// terminate the host in a goroutine. pass the host in as a parameter
		// so that the variable isn't reused for subsequent iterations
		go func(hostToTerminate host.Host) {

			defer waitGroup.Done()

			// wrapper function to terminate the host
			terminateFunc := func() error {
				return terminateHost(&hostToTerminate, mciSettings)
			}

			// run the function with a timeout
			err := util.RunFunctionWithTimeout(terminateFunc, 10*time.Second)

			if err == util.ErrTimedOut {
				errsLock.Lock()
				errors = append(errors, fmt.Errorf("timeout terminating"+
					" host %v", hostToTerminate.Id))
				errsLock.Unlock()
			} else if err != nil {
				errsLock.Lock()
				errors = append(errors, fmt.Errorf("error terminating host:"+
					" %v", err))
				errsLock.Unlock()
			} else {
				evergreen.Logger.Logf(slogger.INFO, "Successfully terminated host"+
					" %v", hostToTerminate.Id)
			}

		}(h)

	}

	// make sure all terminations finish
	waitGroup.Wait()

	return errors
}

// helper to terminate a single host
func terminateHost(host *host.Host, mciSettings *evergreen.MCISettings) error {

	// convert the host to a cloud host
	cloudHost, err := providers.GetCloudHost(host, mciSettings)
	if err != nil {
		return fmt.Errorf("error getting cloud host for %v: %v", host.Id, err)
	}

	// terminate the instance
	if err := cloudHost.TerminateInstance(); err != nil {
		return fmt.Errorf("error terminating host %v: %v", host.Id, err)
	}

	return nil

}
