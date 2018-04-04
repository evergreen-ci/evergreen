package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// ProvisioningCutoff is the threshold to consider as too long for a host to take provisioning
	ProvisioningCutoff = 25 * time.Minute

	// UnreachableCutoff is the threshold to wait for an unreachable host to become marked
	// as reachable again before giving up and terminating it.
	UnreachableCutoff = 5 * time.Minute

	// MaxTimeNextPayment is the amount of time we wait to have left before marking a host as idle
	MaxTimeTilNextPayment = 5 * time.Minute

	// idleTimeCutoff is the amount of time we wait for an idle host to be marked as idle.
	idleTimeCutoff            = 4 * time.Minute
	idleWaitingForAgentCutoff = 10 * time.Minute
)

type hostFlagger struct {
	hostFlaggingFunc
	Reason string
}

// function that takes in all distros - specified as a map of
// distro name -> distro info - as well as the mci settings,
// and spits out a list of hosts to be terminated
type hostFlaggingFunc func(context.Context, []distro.Distro, *evergreen.Settings) ([]host.Host, error)

// flagIdleHosts is a hostFlaggingFunc to get all hosts which have spent too
// long without running a task
func flagIdleHosts(ctx context.Context, d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// will ultimately contain all of the hosts determined to be idle
	idleHosts := []host.Host{}

	// fetch all hosts not currently running a task
	freeHosts, err := host.Find(host.IsFree)
	if err != nil {
		return nil, errors.Wrap(err, "error finding free hosts")
	}

	// go through the hosts, and see if they have idled long enough to
	// be terminated
	for _, freeHost := range freeHosts {
		if !util.StringSliceContains(evergreen.ProviderSpawnable, freeHost.Distro.Provider) {
			// only flag excess hosts for spawnable distros
			continue
		}

		// ask the host how long it has been idle
		idleTime := freeHost.IdleTime()

		// if the communication time is > 10 mins then there may not be an agent on the host.
		communicationTime := freeHost.GetElapsedCommunicationTime()

		// get a cloud manager for the host
		cloudManager, err := cloud.GetCloudManager(ctx, freeHost.Provider, s)
		if err != nil {
			return nil, errors.Wrapf(err, "error getting cloud manager for host %v", freeHost.Id)
		}

		// ask how long until the next payment for the host
		tilNextPayment := cloudManager.TimeTilNextPayment(&freeHost)

		if tilNextPayment > MaxTimeTilNextPayment {
			continue
		}

		if freeHost.IsWaitingForAgent() && (communicationTime < idleWaitingForAgentCutoff || idleTime < idleWaitingForAgentCutoff) {
			grip.Notice(message.Fields{
				"runner":            RunnerName,
				"message":           "not flagging idle host, waiting for an agent",
				"host":              freeHost.Id,
				"distro":            freeHost.Distro.Id,
				"idle":              idleTime.String(),
				"last_communicated": communicationTime.String(),
			})
			continue
		}

		// if we haven't heard from the host or it's been idle for longer than the cutoff, we should flag.
		if communicationTime >= idleTimeCutoff || idleTime >= idleTimeCutoff {
			idleHosts = append(idleHosts, freeHost)
		}
	}

	return idleHosts, nil
}

// flagExcessHosts is a hostFlaggingFunc to get all hosts that push their
// distros over the specified max hosts
func flagExcessHosts(ctx context.Context, distros []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// will ultimately contain all the hosts that can be terminated
	excessHosts := []host.Host{}

	// figure out the excess hosts for each distro
	for _, d := range distros {
		if !util.StringSliceContains(evergreen.ProviderSpawnable, d.Provider) {
			// only flag excess hosts for spawnable distros
			continue
		}

		// fetch any hosts for the distro that count towards max hosts
		allHostsForDistro, err := host.Find(host.ByDistroId(d.Id))
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching hosts for distro %v", d.Id)
		}

		// if there are more than the specified max hosts, then terminate
		// some, if they are not running tasks
		numExcessHosts := len(allHostsForDistro) - d.PoolSize
		if numExcessHosts > 0 {

			// track how many hosts for the distro are terminated
			counter := 0

			for _, host := range allHostsForDistro {
				// if the host is not running a task, it can be
				// safely terminated
				if host.RunningTask == "" {
					excessHosts = append(excessHosts, host)
					counter++
				}

				// break if we've marked enough to be terminated
				if counter == numExcessHosts {
					break
				}
			}
		}

	}
	return excessHosts, nil
}
