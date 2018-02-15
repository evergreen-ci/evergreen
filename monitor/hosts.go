package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
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
func (hm *HostMonitor) RunMonitoringChecks(ctx context.Context, settings *evergreen.Settings) error {
	// used to store any errors that occur
	catcher := grip.NewBasicCatcher()

	for _, f := range hm.monitoringFuncs {
		if ctx.Err() != nil {
			catcher.Add(errors.New("host monitor canceled"))
			break
		}

		// continue on error to allow the other monitoring functions to run
		catcher.Extend(f(settings))
	}

	return catcher.Resolve()
}

// run through the list of host flagging functions, finding all hosts that
// need to be terminated and terminating them
func (hm *HostMonitor) CleanupHosts(ctx context.Context, distros []distro.Distro, settings *evergreen.Settings) error {
	startAt := time.Now()
	catcher := grip.NewBasicCatcher()
	hostIdsToTerminate := []string{}
	hosts := make(chan *host.Host)
	wg := &sync.WaitGroup{}

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case h := <-hosts:
					if h == nil {
						return
					}

					catcher.Add(errors.Wrapf(terminateHost(ctx, h, settings),
						"problem terminating host %s", h.Id))
				}
			}
		}()
	}

	for _, flagger := range hm.flaggingFuncs {
		if ctx.Err() != nil {
			catcher.Add(errors.New("host monitor canceled"))
			break
		}

		grip.Debug(message.Fields{
			"runner":  RunnerName,
			"message": "Running flagging function for hosts",
			"reason":  flagger.Reason,
		})

		// find the next batch of hosts to terminate
		flaggedHosts, err := flagger.hostFlaggingFunc(distros, settings)

		// continuing on error so that one wonky flagging function doesn't
		// stop others from running
		if err != nil {
			catcher.Add(errors.Wrapf(err, "error flagging [%s] hosts to be terminated", flagger.Reason))
			continue
		}

		for _, h := range flaggedHosts {
			if ctx.Err() != nil {
				catcher.Add(errors.New("host monitor canceled"))
				break
			}

			hostIdsToTerminate = append(hostIdsToTerminate, h.Id)
			hosts <- &h
		}
	}
	close(hosts)
	wg.Wait()

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"message":       "terminated flagged hosts",
		"num_hosts":     len(hostIdsToTerminate),
		"num_errors":    catcher.Len(),
		"hosts":         hostIdsToTerminate,
		"duration_secs": time.Since(startAt).Seconds(),
	})

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
	cloudHost, err := cloud.GetCloudHost(ctx, h, settings)
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
	if err := cloudHost.TerminateInstance(ctx, evergreen.User); err != nil {
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
	logs, err := h.RunSSHCommand(ctx, h.TearDownCommand(), sshOptions)
	if err != nil {
		event.LogHostTeardown(h.Id, logs, false, time.Since(startTime))
		return errors.Wrapf(err, "error running teardown script on remote host: %s", logs)
	}
	event.LogHostTeardown(h.Id, logs, true, time.Since(startTime))
	return nil
}
