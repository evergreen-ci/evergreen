package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

// HostAllocator is responsible for determining how many new hosts should be spun up.
type HostAllocator func(context.Context, HostAllocatorData) (int, error)

type HostAllocatorData struct {
	Distro               distro.Distro
	ExistingHosts        []host.Host
	FreeHostFraction     float64
	MaxDurationThreshold time.Duration
	UsesContainers       bool
	ContainerPool        *evergreen.ContainerPool
	DistroQueueInfo      DistroQueueInfo
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
