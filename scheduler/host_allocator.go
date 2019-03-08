package scheduler

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

// HostAllocator is responsible for determining how many new hosts should be spun up.
type HostAllocator func(context.Context, HostAllocatorData) (int, error)

type HostAllocatorData struct {
	Distro           distro.Distro
	ExistingHosts    []host.Host
	FreeHostFraction float64
	UsesContainers   bool
	ContainerPool    *evergreen.ContainerPool
	DistroQueueInfo  model.DistroQueueInfo
}

func GetHostAllocator(name string) HostAllocator {
	switch name {
	case "deficit":
		return DeficitBasedHostAllocator
	case "utilization":
		return UtilizationBasedHostAllocator
	default:
		return UtilizationBasedHostAllocator
	}
}
