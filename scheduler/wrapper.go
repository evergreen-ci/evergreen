package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type Configuration struct {
	DistroID      string
	TaskFinder    string
	HostAllocator string
}

// GetRunnableTasksAndVersions finds tasks whose versions have already been
// created, and returns those tasks, as well as a map of version IDs to versions.
func GetRunnableTasksAndVersions(tasks []task.Task) ([]task.Task, map[string]*version.Version, error) {
	runnableTasks := []task.Task{}
	versions := make(map[string]*version.Version)
	for _, t := range tasks {
		// If we already have the version, the task is runnable. Continue. Append tasks to runnable tasks.
		if _, ok := versions[t.Version]; ok {
			runnableTasks = append(runnableTasks, t)
			continue
		}

		// Otherwise, try to find the version.
		v, err := version.FindOneId(t.Version)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error finding version %s for task %s", t.Id, t.Version)
		}

		// If the version doesn't exist yet, keep going, and do not append tasks to runnableTasks.
		if v == nil {
			continue
		}

		// There was a version, so cache it, and append the task to runnable tasks.
		runnableTasks = append(runnableTasks, t)
		versions[t.Version] = v
	}
	return runnableTasks, versions, nil
}

func PlanDistro(ctx context.Context, conf Configuration) error {
	startAt := time.Now()
	distroSpec, err := distro.FindOne(distro.ById(conf.DistroID))
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	if err = model.UpdateStaticDistro(distroSpec); err != nil {
		return errors.Wrap(err, "problem updating static hosts")
	}

	if err = underwaterUnschedule(conf.DistroID); err != nil {
		return errors.Wrap(err, "problem unscheduling underwater tasks")
	}

	finder := GetTaskFinder(conf.TaskFinder)
	tasks, err := finder(conf.DistroID)
	runnableTasks, versions, err := GetRunnableTasksAndVersions(tasks)
	if err != nil {
		return errors.Wrap(err, "problem calculating task finder")
	}

	projectDurations, err := GetExpectedDurations(runnableTasks)
	if err != nil {
		return errors.Wrap(err, "problem calculating duration")
	}

	ds := &distroSchedueler{
		TaskPrioritizer:    &CmpBasedTaskPrioritizer{},
		TaskQueuePersister: &DBTaskQueuePersister{},
	}

	res := ds.scheduleDistro(conf.DistroID, runnableTasks, versions, projectDurations)
	if res.err != nil {
		return errors.Wrap(res.err, "problem calculating distro plan")
	}

	grip.Info(message.Fields{
		"runner": RunnerName,
		"distro": conf.DistroID,
		"stat":   "distro-queue-size",
		"size":   len(res.taskQueueItem),
	})

	if err = host.RemoveStaleInitializing(conf.DistroID); err != nil {
		return errors.Wrap(err, "problem removing previously intented hosts, before creating new ones.") // nolint:misspell
	}

	distroHostsMap, err := findUsableHosts(conf.DistroID)
	if err != nil {
		return errors.Wrap(err, "with host query")
	}

	allocatorArgs := HostAllocatorData{
		projectTaskDurations: projectDurations,
		taskQueueItems: map[string][]model.TaskQueueItem{
			conf.DistroID: res.taskQueueItem,
		},
		existingDistroHosts: distroHostsMap,
		distros: map[string]distro.Distro{
			conf.DistroID: distroSpec,
		},
	}

	allocator := GetHostAllocator(conf.HostAllocator)
	newHosts, err := allocator(ctx, allocatorArgs)
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	hostsSpawned, err := spawnHosts(ctx, newHosts)
	if err != nil {
		return errors.Wrap(err, "Error spawning new hosts")
	}
	hostList := hostsSpawned[conf.DistroID]

	event.LogSchedulerEvent(event.SchedulerEventData{
		TaskQueueInfo: res.schedulerEvent,
		DistroId:      conf.DistroID,
	})

	var makespan time.Duration
	numHosts := time.Duration(len(distroHostsMap) + len(hostsSpawned))
	if numHosts != 0 {
		makespan = res.schedulerEvent.ExpectedDuration / numHosts
	} else if res.schedulerEvent.TaskQueueLength > 0 {
		makespan = res.schedulerEvent.ExpectedDuration
	}

	grip.Info(message.Fields{
		"message":                "distro-scheduler-report",
		"runner":                 RunnerName,
		"distro":                 conf.DistroID,
		"new_hosts":              hostList,
		"num_hosts":              len(hostList),
		"queue":                  res.schedulerEvent,
		"total_runtime":          res.schedulerEvent.ExpectedDuration.String(),
		"predicted_makespan":     makespan.String(),
		"scheduler_runtime_secs": time.Since(startAt).Seconds(),
	})

	return nil
}
