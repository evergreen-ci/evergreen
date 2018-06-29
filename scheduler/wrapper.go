package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type Configuration struct {
	DistroID         string
	TaskFinder       string
	HostAllocator    string
	FreeHostFraction float64
}

func PlanDistro(ctx context.Context, conf Configuration, s *evergreen.Settings) error {
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
	if err != nil {
		return errors.Wrap(err, "problem calculating task finder")
	}

	runnableTasks, versions, err := filterTasksWithVersionCache(tasks)
	if err != nil {
		return errors.Wrap(err, "error getting runnable tasks")
	}

	ds := &distroSchedueler{
		TaskPrioritizer:    &CmpBasedTaskPrioritizer{},
		TaskQueuePersister: &DBTaskQueuePersister{},
	}

	res := ds.scheduleDistro(conf.DistroID, runnableTasks, versions)
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
		taskQueueItems: map[string][]model.TaskQueueItem{
			conf.DistroID: res.taskQueueItem,
		},
		existingDistroHosts: distroHostsMap,
		distros: map[string]distro.Distro{
			conf.DistroID: distroSpec,
		},
		freeHostFraction: conf.FreeHostFraction,
	}

	// retrieve container pool information for container distros
	var pool *evergreen.ContainerPool
	if distroSpec.ContainerPool != "" {
		pool = s.ContainerPools.GetContainerPool(distroSpec.ContainerPool)
		if pool == nil {
			return errors.Wrap(err, "problem retrieving container pool")
		}
		allocatorArgs.usesContainers = true
		allocatorArgs.containerPool = pool
	}

	allocator := GetHostAllocator(conf.HostAllocator)
	newHosts, err := allocator(ctx, allocatorArgs)
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	hostsSpawned, err := spawnHosts(ctx, newHosts, pool)
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
		"provider":               distroSpec.Provider,
		"max_hosts":              distroSpec.PoolSize,
		"new_hosts":              hostList,
		"num_hosts":              len(hostList),
		"queue":                  res.schedulerEvent,
		"total_runtime":          res.schedulerEvent.ExpectedDuration.String(),
		"predicted_makespan":     makespan.String(),
		"scheduler_runtime_secs": time.Since(startAt).Seconds(),
	})

	return nil
}
