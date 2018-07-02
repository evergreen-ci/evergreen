package scheduler

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

// DeficitBasedHostAllocator decides how many new hosts are needed for a distro by seeing if
// the number of tasks that need to be run for the distro is greater than the number
// of hosts currently free to run a task. Returns a map of distro-># of hosts to spawn.
func DeficitBasedHostAllocator(ctx context.Context, hostAllocatorData HostAllocatorData) (map[string]int, error) {

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

		newHostsNeeded[distroId] = deficitNumNewHostsForDistro(ctx,
			&hostAllocatorData, distro)
	}

	return newHostsNeeded, nil
}

// numNewHostsForDistro determine how many new hosts should be spun up for an
// individual distro
func deficitNumNewHostsForDistro(ctx context.Context,
	hostAllocatorData *HostAllocatorData, distro distro.Distro) int {

	if !distro.IsEphemeral() {
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
