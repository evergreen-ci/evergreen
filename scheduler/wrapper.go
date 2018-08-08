package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
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
	schedulerInstance := util.RandomString()
	startAt := time.Now()
	distroSpec, err := distro.FindOne(distro.ById(conf.DistroID))
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	if err = UpdateStaticDistro(distroSpec); err != nil {
		return errors.Wrap(err, "problem updating static hosts")
	}

	if err = underwaterUnschedule(conf.DistroID); err != nil {
		return errors.Wrap(err, "problem unscheduling underwater tasks")
	}

	startTaskFinder := time.Now()
	finder := GetTaskFinder(conf.TaskFinder)
	tasks, err := finder(conf.DistroID)
	if err != nil {
		return errors.Wrap(err, "problem calculating task finder")
	}
	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        conf.DistroID,
		"operation":     "runtime-stats",
		"phase":         "task-finder",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startTaskFinder).Seconds(),
	})

	runnableTasks, versions, err := filterTasksWithVersionCache(tasks)
	if err != nil {
		return errors.Wrap(err, "error getting runnable tasks")
	}

	ds := &distroSchedueler{
		TaskPrioritizer: &CmpBasedTaskPrioritizer{
			runtimeID: schedulerInstance,
		},
		TaskQueuePersister: &DBTaskQueuePersister{},
		runtimeID:          schedulerInstance,
	}

	startPlanPhase := time.Now()
	res := ds.scheduleDistro(conf.DistroID, runnableTasks, versions)
	if res.err != nil {
		return errors.Wrap(res.err, "problem calculating distro plan")
	}
	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        conf.DistroID,
		"operation":     "runtime-stats",
		"phase":         "planning-distro",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startPlanPhase).Seconds(),

		// The following keys were previously part of a
		// separate message, but to cut down on noise...
		//
		// "runner":   RunnerName,
		// "instance": schedulerInstance,
		// "distro":   conf.DistroID,
		"stat": "distro-queue-size",
		"size": len(res.taskQueueItem),
	})

	startHostAllocation := time.Now()
	if err = host.RemoveStaleInitializing(conf.DistroID); err != nil {
		return errors.Wrap(err, "problem removing previously intented hosts, before creating new ones.") // nolint:misspell
	}

	distroHosts, err := findUsableHosts(conf.DistroID)
	if err != nil {
		return errors.Wrap(err, "with host query")
	}

	allocatorArgs := HostAllocatorData{
		taskQueueItems:   res.taskQueueItem,
		existingHosts:    distroHosts,
		distro:           distroSpec,
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
	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        conf.DistroID,
		"operation":     "runtime-stats",
		"phase":         "host-allocation",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startHostAllocation).Seconds(),
	})

	startHostSpawning := time.Now()
	hostsSpawned, err := spawnHosts(ctx, distroSpec, newHosts, pool)
	if err != nil {
		return errors.Wrap(err, "Error spawning new hosts")
	}

	event.LogSchedulerEvent(event.SchedulerEventData{
		TaskQueueInfo: res.schedulerEvent,
		DistroId:      conf.DistroID,
	})

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        conf.DistroID,
		"operation":     "runtime-stats",
		"phase":         "host-spawning",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startHostSpawning).Seconds(),
	})

	var makespan time.Duration
	if len(hostsSpawned) != 0 {
		makespan = res.schedulerEvent.ExpectedDuration / time.Duration(len(hostsSpawned))
	} else if res.schedulerEvent.TaskQueueLength > 0 {
		makespan = res.schedulerEvent.ExpectedDuration
	}

	grip.Info(message.Fields{
		"message":                "distro-scheduler-report",
		"runner":                 RunnerName,
		"distro":                 conf.DistroID,
		"provider":               distroSpec.Provider,
		"max_hosts":              distroSpec.PoolSize,
		"new_hosts":              hostsSpawned,
		"num_hosts":              len(hostsSpawned),
		"queue":                  res.schedulerEvent,
		"total_runtime":          res.schedulerEvent.ExpectedDuration.String(),
		"predicted_makespan":     makespan.String(),
		"scheduler_runtime_secs": time.Since(startAt).Seconds(),
		"instance":               schedulerInstance,
	})

	return nil
}

func UpdateStaticDistro(d distro.Distro) error {
	if d.Provider != evergreen.ProviderNameStatic {
		return nil
	}

	hosts, err := doStaticHostUpdate(d)
	if err != nil {
		return errors.WithStack(err)
	}

	if d.Id == "" || len(hosts) == 0 {
		return nil
	}

	return host.MarkInactiveStaticHosts(hosts, d.Id)
}

func doStaticHostUpdate(d distro.Distro) ([]string, error) {
	settings := &cloud.StaticSettings{}
	err := mapstructure.Decode(d.ProviderSettings, settings)
	if err != nil {
		return nil, errors.Errorf("invalid static settings for '%v'", d.Id)
	}

	staticHosts := []string{}
	for _, h := range settings.Hosts {
		hostInfo, err := util.ParseSSHInfo(h.Name)
		if err != nil {
			return nil, err
		}
		user := hostInfo.User
		if user == "" {
			user = d.User
		}
		staticHost := host.Host{
			Id:           h.Name,
			User:         user,
			Host:         h.Name,
			Distro:       d,
			CreationTime: time.Now(),
			StartedBy:    evergreen.User,
			Status:       evergreen.HostRunning,
			Provisioned:  true,
		}

		if d.Provider == evergreen.ProviderNameStatic {
			staticHost.Provider = evergreen.HostTypeStatic
		}

		// upsert the host
		_, err = staticHost.Upsert()
		if err != nil {
			return nil, err
		}
		staticHosts = append(staticHosts, h.Name)
	}

	return staticHosts, nil
}
