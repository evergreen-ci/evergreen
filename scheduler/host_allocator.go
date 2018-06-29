package scheduler

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
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
type HostAllocator func(context.Context, HostAllocatorData) (map[string]int, error)

// HostAllocatorData is the set of parameters passed to a HostAllocator.
type HostAllocatorData struct {
	taskQueueItems      map[string][]model.TaskQueueItem
	existingDistroHosts map[string][]host.Host
	taskRunDistros      map[string][]string
	distros             map[string]distro.Distro
	freeHostFraction    float64
	usesContainers      bool
	containerPool       *evergreen.ContainerPool
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
		return DurationBasedHostAllocator
	}
}
