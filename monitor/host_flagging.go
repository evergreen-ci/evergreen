package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
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
)

type hostFlagger struct {
	hostFlaggingFunc
	Reason string
}

// function that takes in all distros - specified as a map of
// distro name -> distro info - as well as the mci settings,
// and spits out a list of hosts to be terminated
type hostFlaggingFunc func(context.Context, []distro.Distro, *evergreen.Settings) ([]host.Host, error)

// flagExcessHosts is a hostFlaggingFunc to get all hosts that push their
// distros over the specified max hosts
func flagExcessHosts(ctx context.Context, distros []distro.Distro, s *evergreen.Settings) ([]host.Host, error) {
	// will ultimately contain all the hosts that can be terminated
	excessHosts := []host.Host{}

	// figure out the excess hosts for each distro
	for _, d := range distros {
		if !d.IsEphemeral() {
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

			grip.Notice(message.Fields{
				"runner":        RunnerName,
				"message":       "found excess hosts",
				"distro":        d.Id,
				"max_hosts":     d.PoolSize,
				"running_hosts": len(allHostsForDistro),
				"idle":          counter,
				"excess":        numExcessHosts,
			})
		}
	}

	return excessHosts, nil
}
