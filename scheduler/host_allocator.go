package scheduler

import (
	"context"
)

// Re-define the arguments' dscription

// HostAllocator is responsible for determining how many new hosts should be spun up.
// Parameters:
//  taskQueueItems: a map of distro name -> task queue items for that distro (a TaskQueue object)
//  distros: a map of distro name -> information on that distro (a model.Distro object)
//  existingDistroHosts: a map of distro name -> currently running hosts on that distro
//  projectTaskDurations: the expected duration of tasks by project and variant
//  taskRunDistros: a map of task id -> distros the task is allowed to run on
// Returns a map of distro name -> how many hosts need to be spun up for that distro.
type HostAllocator func(context.Context, HostAllocatorData) (int, error)

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
