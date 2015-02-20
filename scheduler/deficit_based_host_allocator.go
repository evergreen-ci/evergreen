package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
)

// Implementation, that uses the difference between the number of free hosts
// and the number of tasks that need to be run as a metric for how many new
// hosts need to be spun up
type DeficitBasedHostAllocator struct{}

// Implementation of NewHostsNeeded.  Decides that new hosts are needed for a
// distro if the number of tasks that need to be run for the distro is greater
// than the number of hosts currently free to run a task.
func (self *DeficitBasedHostAllocator) NewHostsNeeded(
	hostAllocatorData HostAllocatorData, mciSettings *mci.MCISettings) (map[string]int, error) {

	newHostsNeeded := make(map[string]int)

	// now, for each distro, see if we need to spin up any new hosts
	for distroName, _ := range hostAllocatorData.taskQueueItems {
		distro, ok := hostAllocatorData.distros[distroName]
		if !ok {
			return nil, fmt.Errorf("No distro info available for distro %v",
				distroName)
		}
		if distro.Name != distroName {
			return nil, fmt.Errorf("Bad mapping between task queue distro "+
				"name and host allocator distro data: %v != %v", distro.Name,
				distroName)
		}

		newHostsNeeded[distroName] = self.numNewHostsForDistro(
			&hostAllocatorData, distro, mciSettings)
	}

	return newHostsNeeded, nil
}

// numNewHostsForDistro determine how many new hosts should be spun up for an
// individual distro
func (self *DeficitBasedHostAllocator) numNewHostsForDistro(
	hostAllocatorData *HostAllocatorData, distro model.Distro, mciSettings *mci.MCISettings) int {

	cloudManager, err := providers.GetCloudManager(distro.Provider, mciSettings)

	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "Couldn't get cloud manager for distro %v with provider %v: %v",
			distro.Name, distro.Provider, err)
		return 0
	}

	can, err := cloudManager.CanSpawn()
	if err != nil {
		mci.Logger.Logf(slogger.ERROR, "Couldn't check if cloud provider %v is spawnable: %v",
			distro.Provider, err)
		return 0
	}
	if !can {
		return 0
	}

	existingDistroHosts := hostAllocatorData.existingDistroHosts[distro.Name]
	runnableDistroTasks := hostAllocatorData.
		taskQueueItems[distro.Name]

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
		distro.MaxHosts-len(existingDistroHosts),
	)

	// cap to zero as lower bound
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return numNewHosts
}
