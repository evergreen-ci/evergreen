package scheduler

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// DeficitBasedHostAllocator uses the difference between the number of free hosts
// and the number of tasks that need to be run as a metric for how many new
// hosts need to be spun up
type DeficitBasedHostAllocator struct{}

// NewHostsNeeded decides how many new hosts are needed for a distro by seeing if
// the number of tasks that need to be run for the distro is greater than the number
// of hosts currently free to run a task. Returns a map of distro-># of hosts to spawn.
func (self *DeficitBasedHostAllocator) NewHostsNeeded(
	hostAllocatorData HostAllocatorData, settings *evergreen.Settings) (map[string]int, error) {

	newHostsNeeded := make(map[string]int)

	// now, for each distro, see if we need to spin up any new hosts
	for distroId := range hostAllocatorData.taskQueueItems {
		distro, ok := hostAllocatorData.distros[distroId]
		if !ok {
			return nil, errors.Errorf("No distro info available for distro %v",
				distroId)
		}
		if distro.Id != distroId {
			return nil, errors.Errorf("Bad mapping between task queue distro "+
				"name and host allocator distro data: %v != %v", distro.Id,
				distroId)
		}

		newHostsNeeded[distroId] = self.numNewHostsForDistro(
			&hostAllocatorData, distro, settings)
	}

	return newHostsNeeded, nil
}

// numNewHostsForDistro determine how many new hosts should be spun up for an
// individual distro
func (self *DeficitBasedHostAllocator) numNewHostsForDistro(
	hostAllocatorData *HostAllocatorData, distro distro.Distro, settings *evergreen.Settings) int {

	cloudManager, err := cloud.GetCloudManager(distro.Provider, settings)

	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "could not get cloud provider for distro",
			"distro":   distro.Id,
			"provider": distro.Provider,
			"runner":   RunnerName,
		}))
		return 0
	}

	can, err := cloudManager.CanSpawn()
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"distro":   distro.Id,
			"provider": distro.Provider,
			"runner":   RunnerName,
			"message":  "could not check if provider is spawnable",
		}))
		return 0
	}
	if !can {
		return 0
	}

	existingDistroHosts := hostAllocatorData.existingDistroHosts[distro.Id]
	runnableDistroTasks := hostAllocatorData.taskQueueItems[distro.Id]

	freeHosts := make([]host.Host, 0, len(existingDistroHosts))
	for _, existingDistroHost := range existingDistroHosts {
		if existingDistroHost.RunningTask == "" {
			freeHosts = append(freeHosts, existingDistroHost)
		}
	}

	numNewHosts := util.Min(
		// the deficit of available hosts vs. tasks to be run
		len(runnableDistroTasks)-len(freeHosts),
		// the maximum number of new hosts we're allowed to spin up
		distro.PoolSize-len(existingDistroHosts),
	)

	// cap to zero as lower bound
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return numNewHosts
}
