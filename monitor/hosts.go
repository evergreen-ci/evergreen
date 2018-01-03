package monitor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/notify"
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

	return errs
}

// run through the list of host flagging functions, finding all hosts that
// need to be terminated and terminating them
func (hm *HostMonitor) CleanupHosts(ctx context.Context, distros []distro.Distro, settings *evergreen.Settings) []error {

	// used to store any errors that occur
	var errs []error

	for _, flagger := range hm.flaggingFuncs {
		if ctx.Err() != nil {
			return append(errs, errors.New("host monitor canceled"))
		}

		grip.Info(message.Fields{
			"runner":  RunnerName,
			"message": "Running flagging function for hosts",
			"reason":  flagger.Reason,
		})
		// find the next batch of hosts to terminate
		hostsToTerminate, err := flagger.hostFlaggingFunc(distros, settings)
		hostIdsToTerminate := []string{}
		for _, h := range hostsToTerminate {
			hostIdsToTerminate = append(hostIdsToTerminate, h.Id)
		}
		grip.Info(message.Fields{
			"runner":  RunnerName,
			"message": "found hosts for termination",
			"reason":  flagger.Reason,
			"count":   len(hostIdsToTerminate),
			"hosts":   hostIdsToTerminate,
		})
		// continuing on error so that one wonky flagging function doesn't
		// stop others from running
		if err != nil {
			errs = append(errs, errors.Wrap(err, "error flagging hosts to be terminated"))
			continue
		}

		// terminate all of the dead hosts. continue on error to allow further
		// termination to work
		if err = terminateHosts(ctx, hostsToTerminate, settings, flagger.Reason); err != nil {
			errs = append(errs, errors.Wrap(err, "error terminating host"))
			continue
		}

		grip.Info(message.Fields{
			"runner":  RunnerName,
			"message": "terminated flagged hosts",
			"reason":  flagger.Reason,
			"count":   len(hostIdsToTerminate),
			"hosts":   hostIdsToTerminate,
		})
	}

	return errs
}

// terminate the passed-in slice of hosts. returns any errors that occur
// terminating the hosts
func terminateHosts(ctx context.Context, hosts []host.Host, settings *evergreen.Settings, reason string) error {
	catcher := grip.NewBasicCatcher()
	work := make(chan *host.Host, len(hosts))
	wg := &sync.WaitGroup{}

	// The naive range case does not work with pointers https://play.golang.org/p/JL17Ah7HdU.
	for i := range hosts {
		work <- &hosts[i]
	}
	close(work)

	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for hostToTerminate := range work {
				event.LogMonitorOperation(hostToTerminate.Id, reason)

				func() { // use a function so that the defer works
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
					defer cancel()

					if err := terminateHost(ctx, hostToTerminate, settings); err != nil {
						if strings.Contains(err.Error(), cloud.EC2ErrorNotFound) {
							err = hostToTerminate.Terminate()
							if err != nil {
								catcher.Add(errors.Wrap(err, "unable to set host as terminated"))
								return
							}
							grip.Debug(message.Fields{
								"runner":  RunnerName,
								"message": "host not found in EC2, changed to terminated",
								"host":    hostToTerminate.Id,
							})
							return
						}

						catcher.Add(errors.Wrapf(err, "error terminating host %s", hostToTerminate.Id))
					}
				}()
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
		grip.Warning(message.Fields{
			"runner":  RunnerName,
			"message": "Host has running task; clearing before terminating",
			"host":    h.Id,
			"task":    h.RunningTask,
		})
		err := h.ClearRunningTask(h.RunningTask, time.Now())
		if err != nil {
			grip.Error(message.Fields{
				"runner":  RunnerName,
				"message": "Error clearing running task for host",
				"host":    h.Id,
			})
		}
	}
	// convert the host to a cloud host
	cloudHost, err := cloud.GetCloudHost(h, settings)
	if err != nil {
		return errors.Wrapf(err, "error getting cloud host for %v", h.Id)
	}

	// run teardown script if we have one, sending notifications if things go awry
	if h.Distro.Teardown != "" && h.Provisioned {
		grip.Info(message.Fields{
			"runner":  RunnerName,
			"message": "running teardown script for host",
			"host":    h.Id,
		})

		if err := runHostTeardown(ctx, h, cloudHost); err != nil {
			grip.Error(message.Fields{
				"runner":  RunnerName,
				"message": "Error running teardown script",
				"host":    h.Id,
			})

			subj := fmt.Sprintf("%v Error running teardown for host %v",
				notify.TeardownFailurePreface, h.Id)

			grip.Error(message.WrapError(notify.NotifyAdmins(subj, err.Error(), settings),
				message.Fields{
					"message": "problem sending email",
					"host":    h.Id,
					"subject": subj,
					"error":   err.Error(),
					"runner":  RunnerName,
				}))
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
	// run the teardown script with the agent
	logs, err := hostutil.RunSSHCommand(ctx, hostutil.TearDownCommand(h), sshOptions, *h)
	if err != nil {
		event.LogHostTeardown(h.Id, logs, false, time.Since(startTime))
		return errors.Wrapf(err, "error running teardown script on remote host: %s", logs)
	}
	event.LogHostTeardown(h.Id, logs, true, time.Since(startTime))
	return nil
}
