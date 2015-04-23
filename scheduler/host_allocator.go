package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
)

// Interface responsible for determining how many new hosts should be spun up.
// Parameters:
//
//  distros: a map of distro name -> information on that distro (a model.Distro
//      object)
//  existingDistroHosts: a map of distro name -> currently running hosts on that
//      distro
//  taskQueueItems: a map of distro name -> task queue items for that distro (a
//		TaskQueue object)
//  projectTaskDurations: the expected duration of tasks by project, project and
// 		buildvariant - gotten from the db
//  taskRunDistros: a map of task id -> distros the task is allowed to run on
// Returns a map of distro name -> how many hosts need to be spun up for that
//      distro
//
type HostAllocator interface {
	NewHostsNeeded(allocatorData HostAllocatorData,
		mciSettings *mci.MCISettings) (map[string]int, error)
}

type HostAllocatorData struct {
	taskQueueItems       map[string][]model.TaskQueueItem
	existingDistroHosts  map[string][]model.Host
	taskRunDistros       map[string][]string
	distros              map[string]model.Distro
	projectTaskDurations model.ProjectTaskDurations
}
