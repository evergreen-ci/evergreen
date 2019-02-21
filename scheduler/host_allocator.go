package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

// HostAllocator is responsible for determining how many new hosts should be spun up.
// Parameters:
//  taskQueueItems: a map of distro name -> task queue items for that distro (a TaskQueue object)
//  distros: a map of distro name -> information on that distro (a model.Distro object)
//  existingDistroHosts: a map of distro name -> currently running hosts on that distro
//  projectTaskDurations: the expected duration of tasks by project and variant
//  taskRunDistros: a map of task id -> distros the task is allowed to run on
// Returns a map of distro name -> how many hosts need to be spun up for that distro.
type HostAllocator func(context.Context, HostAllocatorData) (int, error)

type HostAllocatorData struct {
	Distro               distro.Distro
	ExistingHosts        []host.Host
	FreeHostFraction     float64
	MaxDurationThreshold time.Duration
	UsesContainers       bool
	ContainerPool        *evergreen.ContainerPool
	DistroQueueInfo      DistroQueueInfo
	TaskRunDistros       []string // used by duration_based_host_allocator
}

func GetHostAllocator(name string) HostAllocator {
	switch name {
	case "deficit":
		return DeficitBasedHostAllocator
	case "duration":
		return DurationBasedHostAllocator
	case "utilization":
		return UtilizationBasedHostAllocator
	default:
		return UtilizationBasedHostAllocator
	}
}
