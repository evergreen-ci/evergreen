package monitor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/cloud/providers/ec2"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
func (hm *HostMonitor) RunMonitoringChecks(ctx context.Context, settings *evergreen.Settings) []error {
	grip.Info("Running host monitoring checks...")

	// used to store any errors that occur
	var errs []error

	for _, f := range hm.monitoringFuncs {
		if ctx.Err() != nil {
			return append(errs, errors.New("host monitor canceled"))
		}

		// continue on error to allow the other monitoring functions to run
		if flagErrs := f(settings); errs != nil {
			errs = append(errs, flagErrs...)
		}
	}

	grip.Info("Finished running host monitoring checks")

	return errs

}

// run through the list of host flagging functions, finding all hosts that
// need to be terminated and terminating them
func (hm *HostMonitor) CleanupHosts(ctx context.Context, distros []distro.Distro, settings *evergreen.Settings) []error {

	grip.Info("Running host cleanup...")

	// used to store any errors that occur
	var errs []error

	for idx, flagger := range hm.flaggingFuncs {
		if ctx.Err() != nil {
			return append(errs, errors.New("host monitor canceled"))
		}

		grip.Infoln("Searching for flagged hosts under criteria:", flagger.Reason)
		// find the next batch of hosts to terminate
		hostsToTerminate, err := flagger.hostFlaggingFunc(distros, settings)
		grip.Infof("Found %v hosts flagged for '%v'", len(hostsToTerminate), flagger.Reason)

		// continuing on error so that one wonky flagging function doesn't
		// stop others from running
		if err != nil {
			errs = append(errs, errors.Wrap(err, "error flagging hosts to be terminated"))
			continue
		}

		grip.Infof("Check %v: found %v hosts to be terminated", idx, len(hostsToTerminate))

		// terminate all of the dead hosts. continue on error to allow further
		// termination to work
		if err = terminateHosts(ctx, hostsToTerminate, settings, flagger.Reason); err != nil {
			errs = append(errs, errors.Wrap(err, "error terminating host"))
			continue
		}
	}

	return errs
}

// terminate the passed-in slice of hosts. returns any errors that occur
// terminating the hosts
func terminateHosts(ctx context.Context, hosts []host.Host, settings *evergreen.Settings, reason string) error {
	catcher := grip.NewBasicCatcher()
	work := make(chan *host.Host, len(hosts))
	wg := &sync.WaitGroup{}

	for _, h := range hosts {
		work <- &h
	}
	close(work)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for hostToTerminate := range work {
				event.LogMonitorOperation(hostToTerminate.Id, reason)

				err := util.RunFunctionWithTimeout(func() error {
					return terminateHost(ctx, hostToTerminate, settings)
				}, 12*time.Minute)

				if err != nil {
					if strings.Contains(err.Error(), ec2.EC2ErrorNotFound) {
						err = hostToTerminate.Terminate()
						if err != nil {
							catcher.Add(errors.Wrap(err, "unable to set host as terminated"))
							continue
						}
						grip.Debugf("host %s not found in EC2, changed to terminated", hostToTerminate.Id)
						continue
					}
					if err == util.ErrTimedOut {
						catcher.Add(errors.Errorf("timeout terminating host %s", hostToTerminate.Id))
						continue
					}
					err = errors.Wrapf(err, "error terminating host %s", hostToTerminate.Id)
					catcher.Add(err)
					grip.Warning(err)
					continue
				}

				grip.Infoln("Successfully terminated host", hostToTerminate.Id)
			}
		}()
	}

	wg.Wait()

	return catcher.Resolve()
}

// helper to terminate a single host
func terminateHost(ctx context.Context, h *host.Host, settings *evergreen.Settings) error {
	// clear the running task of the host in case one has been assigned.
	if h.RunningTask != "" {
		grip.Warningf("Host has running task: %s. Clearing running task field for host"+
			"before terminating.", h.RunningTask)
		err := h.ClearRunningTask(h.RunningTask, time.Now())
		if err != nil {
			grip.Errorf("Error clearing running task for host: %s", h.Id)
		}
	}
	// convert the host to a cloud host
	cloudHost, err := providers.GetCloudHost(h, settings)
	if err != nil {
		return errors.Wrapf(err, "error getting cloud host for %v", h.Id)
	}

	// run teardown script if we have one, sending notifications if things go awry
	if h.Distro.Teardown != "" && h.Provisioned {
		grip.Error(message.Fields{
			"message": "running teardown script for host",
			"host":    h.Id,
		})

		if err := runHostTeardown(ctx, h, cloudHost); err != nil {
			grip.Error(errors.Wrapf(err, "Error running teardown script for %s", h.Id))

			subj := fmt.Sprintf("%v Error running teardown for host %v",
				notify.TeardownFailurePreface, h.Id)

			grip.Error(errors.Wrap(notify.NotifyAdmins(subj, err.Error(), settings),
				"Error sending email"))
		}
	}

	// terminate the instance
	if err := cloudHost.TerminateInstance(); err != nil {
		return errors.Wrapf(err, "error terminating host %s", h.Id)
	}

	return nil
}

func runHostTeardown(ctx context.Context, h *host.Host, cloudHost *cloud.CloudHost) error {
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "error getting ssh options for host %s", h.Id)
	}
	startTime := time.Now()
	logs, err := hostutil.RunRemoteScript(ctx, h, "teardown.sh", sshOptions)
	event.LogHostTeardown(h.Id, logs, err == nil, time.Since(startTime))
	if err != nil {
		return errors.Wrapf(err, "error running teardown.sh over ssh: %v", logs)
	}
	return nil
}
