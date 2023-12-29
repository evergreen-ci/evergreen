package scheduler

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

// HostAllocator is responsible for determining how many new hosts should be
// spun up. The first returned int is this number, and the second returned int
// is the rough number of free hosts.
type HostAllocator func(context.Context, *HostAllocatorData) (newHostsNeeded int, estimatedFreeHosts int, err error)

type HostAllocatorData struct {
	Distro          distro.Distro
	ExistingHosts   []host.Host
	UsesContainers  bool
	ContainerPool   *evergreen.ContainerPool
	DistroQueueInfo model.DistroQueueInfo
}

func GetHostAllocator(name string) HostAllocator {
	switch name {
	case evergreen.HostAllocatorUtilization:
		return UtilizationBasedHostAllocator
	default:
		return UtilizationBasedHostAllocator
	}
}
