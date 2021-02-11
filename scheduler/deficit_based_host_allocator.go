package scheduler

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
)

// DeficitBasedHostAllocator decides how many new hosts are needed for a distro by seeing if
// the number of tasks that need to be run for the distro is greater than the number
// of hosts currently free to run a task. Returns a map of distro-># of hosts to spawn.
func DeficitBasedHostAllocator(ctx context.Context, hostAllocatorData HostAllocatorData) (int, int, error) {
	needed, free := deficitNumNewHostsForDistro(ctx, &hostAllocatorData, hostAllocatorData.Distro)
	return needed, free, nil
}

// numNewHostsForDistro determine how many new hosts should be spun up for an
// individual distro
func deficitNumNewHostsForDistro(ctx context.Context, hostAllocatorData *HostAllocatorData, distro distro.Distro) (int, int) {
	if !distro.IsEphemeral() {
		return 0, 0
	}

	freeHosts := make([]host.Host, 0, len(hostAllocatorData.ExistingHosts))
	for _, existingDistroHost := range hostAllocatorData.ExistingHosts {
		if existingDistroHost.RunningTask == "" {
			freeHosts = append(freeHosts, existingDistroHost)
		}
	}

	numNewHosts := util.Min(
		// the deficit of available hosts vs. tasks to be run
		hostAllocatorData.DistroQueueInfo.Length-len(freeHosts),
		// the maximum number of new hosts we're allowed to spin up
		distro.HostAllocatorSettings.MaximumHosts-len(hostAllocatorData.ExistingHosts),
	)

	// cap to zero as lower bound
	if numNewHosts < 0 {
		numNewHosts = 0
	}

	return numNewHosts, len(freeHosts)
}
