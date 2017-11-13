package monitor

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
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
	idleTimeCutoff            = 7 * time.Minute
	idleWaitingForAgentCutoff = 10 * time.Minute
)

type hostFlagger struct {
	hostFlaggingFunc
	Reason string
}

// function that takes in all distros - specified as a map of
// distro name -> distro info - as well as the mci settings,
// and spits out a list of hosts to be terminated
type hostFlaggingFunc func([]distro.Distro, *evergreen.Settings) ([]host.Host, error)

// flagDecommissionedHosts is a hostFlaggingFunc to get all hosts which should
// be terminated because they are decommissioned
func flagDecommissionedHosts(d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	hosts, err := host.Find(host.IsDecommissioned)
	if err != nil {
		return nil, errors.Wrap(err, "error finding decommissioned hosts")
	}
	return hosts, nil
}

// flagUnreachableHosts is a hostFlaggingFunc to get all hosts which should
// be terminated because they are unreachable
func flagUnreachableHosts(d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	threshold := time.Now().Add(-1 * UnreachableCutoff)
	hosts, err := host.Find(host.ByUnreachableBefore(threshold))
	if err != nil {
		return nil, errors.Wrapf(err, "error finding hosts unreachable since before %v", threshold)
	}

	unreachables := []host.Host{}
	for _, host := range hosts {
		canTerminate, err := hostCanBeTerminated(host, s)
		if err != nil {
			return nil, errors.Wrapf(err, "error checking if host %v can be terminated", host.Id)
		}

		if canTerminate {
			unreachables = append(unreachables, host)
		}
	}

	return unreachables, nil
}

// flagIdleHosts is a hostFlaggingFunc to get all hosts which have spent too
// long without running a task
func flagIdleHosts(d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
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

		// ask the host how long it has been idle
		idleTime := freeHost.IdleTime()

		// if the communication time is > 10 mins then there may not be an agent on the host.
		communicationTime := freeHost.GetElapsedCommunicationTime()

		// get a cloud manager for the host
		cloudManager, err := providers.GetCloudManager(freeHost.Provider, s)
		if err != nil {
			return nil, errors.Wrapf(err, "error getting cloud manager for host %v", freeHost.Id)
		}

		// if the host is not dynamically spun up (and can thus be terminated),
		// skip it
		canTerminate, err := hostCanBeTerminated(freeHost, s)
		if err != nil {
			return nil, errors.Wrapf(err, "error checking if host %v can be terminated", freeHost.Id)
		}
		if !canTerminate {
			continue
		}

		// ask how long until the next payment for the host
		tilNextPayment := cloudManager.TimeTilNextPayment(&freeHost)

		if tilNextPayment > MaxTimeTilNextPayment {
			continue
		}

		if freeHost.IsWaitingForAgent() && (communicationTime >= idleWaitingForAgentCutoff || idleTime >= idleWaitingForAgentCutoff) {
			grip.Warning(message.Fields{
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
func flagExcessHosts(distros []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// will ultimately contain all the hosts that can be terminated
	excessHosts := []host.Host{}

	// figure out the excess hosts for each distro
	for _, d := range distros {

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

				// if the host is not dynamically spun up (and can
				// thus be terminated), skip it
				canTerminate, err := hostCanBeTerminated(host, s)
				if err != nil {
					return nil, errors.Wrapf(err, "error checking if host %s can be terminated", host.Id)
				}
				if !canTerminate {
					continue
				}

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

// flagUnprovisionedHosts is a hostFlaggingFunc to get all hosts that are
// taking too long to provision
func flagUnprovisionedHosts(d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// fetch all hosts that are taking too long to provision
	threshold := time.Now().Add(-ProvisioningCutoff)
	hosts, err := host.Find(host.ByUnprovisionedSince(threshold))
	if err != nil {
		return nil, errors.Wrap(err, "error finding unprovisioned hosts")
	}
	return hosts, err
}

// flagProvisioningFailedHosts is a hostFlaggingFunc to get all hosts
// whose provisioning failed
func flagProvisioningFailedHosts(d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// fetch all hosts whose provisioning failed
	hosts, err := host.Find(host.IsProvisioningFailure)
	if err != nil {
		return nil, errors.Wrap(err, "error finding hosts whose provisioning failed")
	}
	return hosts, nil

}

// flagExpiredHosts is a hostFlaggingFunc to get all user-spawned hosts
// that have expired
func flagExpiredHosts(d []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// fetch the expired hosts
	hosts, err := host.Find(host.ByExpiredSince(time.Now()))
	if err != nil {
		return nil, errors.Wrap(err, "error finding expired spawned hosts")
	}
	return hosts, nil

}

// helper to check if a host can be terminated
func hostCanBeTerminated(h host.Host, s *evergreen.Settings) (bool, error) {
	// get a cloud manager for the host
	cloudManager, err := providers.GetCloudManager(h.Provider, s)
	if err != nil {
		return false, errors.Wrapf(err, "error getting cloud manager for host %s", h.Id)
	}

	// if the host is not part of a spawnable distro, then it was not
	// dynamically spun up and as such cannot be terminated
	canSpawn, err := cloudManager.CanSpawn()
	if err != nil {
		return false, errors.Wrapf(err, "error checking if cloud manager for host %s supports spawning")
	}

	return canSpawn, nil

}
