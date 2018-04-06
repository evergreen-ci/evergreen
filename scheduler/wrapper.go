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

type SchedulerRuntimeConfig struct {
	DistroID    string
	TaskFinder  string
	evgSettings *evergreen.Settings
}

func ScheduleDistro(ctx context.Context, conf SchedulerRuntimeConfig) error {
	startAt := time.Now()
	distroSpec, err := distro.FindOne(distro.ById(conf.DistroID))
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	if err = model.UpdateStaticDistro(distroSpec); err != nil {
		return errors.Wrap(err, "problem updating static hosts")
	}

	finder := GetTaskFinder(conf.TaskFinder)
	runnableTasks, err := finder(conf.DistroID)
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

	res := ds.scheduleDistro(conf.DistroID, runnableTasks, projectDurations)
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

	hs := &hostScheduler{
		Settings:      conf.evgSettings,
		HostAllocator: &DurationBasedHostAllocator{},
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

	newHosts, err := hs.NewHostsNeeded(ctx, allocatorArgs)
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	hostsSpawned, err := hs.spawnHosts(ctx, newHosts)
	if err != nil {
		return errors.Wrap(err, "Error spawning new hosts")
	}
	hostList := hostsSpawned[conf.DistroID]

	event.LogSchedulerEvent(event.SchedulerEventData{
		TaskQueueInfo: res.schedulerEvent,
		DistroId:      conf.DistroID,
	})

	makespan := res.schedulerEvent.ExpectedDuration / time.Duration(len(distroHostsMap)+len(hostsSpawned))
	grip.Info(message.Fields{
		"message":                "hosts spawned",
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
