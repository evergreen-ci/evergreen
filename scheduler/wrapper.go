package scheduler

import (
	"context"
	"fmt"
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
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

type DistroQueueInfo struct {
	Distro             distro.Distro
	Length             int
	ExpectedDuration   time.Duration
	CountOverThreshold int
	TaskGroupsInfo     map[string]TaskGroupInfo
}

type TaskGroupInfo struct {
	GroupID               string
	Count                 int
	MaxHosts              int
	ExpectedDuration      time.Duration
	CountOverThreshold    int
	DurationOverThreshold time.Duration
}

type HostAllocatorData struct {
	Distro           distro.Distro
	ExistingHosts    []host.Host
	TaskRunDistros   []string // Is this actually used?
	FreeHostFraction float64
	UsesContainers   bool
	ContainerPool    *evergreen.ContainerPool
	DistroQueueInfo  DistroQueueInfo
}

type Configuration struct {
	DistroID         string
	TaskFinder       string
	HostAllocator    string
	FreeHostFraction float64
}

func PlanDistro(ctx context.Context, conf Configuration, s *evergreen.Settings) error {
	schedulerInstance := util.RandomString()
	startAt := time.Now()
	distro, err := distro.FindOne(distro.ById(conf.DistroID))
	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	if err = UpdateStaticDistro(distro); err != nil {
		return errors.Wrap(err, "problem updating static hosts")
	}

	if err = underwaterUnschedule(distro.Id); err != nil {
		return errors.Wrap(err, "problem unscheduling underwater tasks")
	}

	if distro.Disabled {
		grip.InfoWhen(sometimes.Quarter(), message.Fields{
			"message": "scheduling for distro is disabled",
			"runner":  RunnerName,
			"distro":  distro.Id,
		})
		return nil
	}

	startTaskFinder := time.Now()
	finder := GetTaskFinder(conf.TaskFinder)
	tasks, err := finder(distro.Id)
	if err != nil {
		return errors.Wrap(err, "problem calculating task finder")
	}
	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
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
	res := ds.scheduleDistro(distro.Id, runnableTasks, versions)
	if res.err != nil {
		return errors.Wrap(res.err, "problem calculating distro plan")
	}
	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
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
	if err = host.RemoveStaleInitializing(distro.Id); err != nil {
		return errors.Wrap(err, "problem removing previously intented hosts, before creating new ones.") // nolint:misspell
	}

	distroHosts, err := host.AllRunningHosts(distro.Id)

	///////////////////////////////////////////////////////////////////////
	// allocatorArgs := HostAllocatorData{
	// 	taskQueueItems:   res.taskQueueItem, // []model.TaskQueueItem from distroSchedulerResult.taskQueueItem retuned by ds.scheduleDistro()
	// 	existingHosts:    distroHosts,       // distroHosts, err := host.AllRunningHosts(conf.DistroID)
	// 	distro:           distroSpec,        // distroSpec, err := distro.FindOne(distro.ById(conf.DistroID))
	// 	freeHostFraction: conf.FreeHostFraction,
	// }

	allocatorData := HostAllocatorData{
		Distro:           distro,
		ExistingHosts:    distroHosts, // distroHosts, err := host.AllRunningHosts(conf.DistroID)
		FreeHostFraction: conf.FreeHostFraction,
	}

	// retrieve container pool information for container distros
	var pool *evergreen.ContainerPool
	if distro.ContainerPool != "" {
		pool = s.ContainerPools.GetContainerPool(distro.ContainerPool)
		if pool == nil {
			return errors.Wrap(err, "problem retrieving container pool")
		}
		allocatorData.UsesContainers = true
		allocatorData.ContainerPool = pool
	}

	///////////////////////////////////////////////////////////////////////
	// STU AT WORK!

	durationThreshold := MaxDurationPerDistroHost
	if allocatorData.UsesContainers {
		durationThreshold = MaxDurationPerDistroHostWithContainers
	}

	taskGroupsInfo := map[string]TaskGroupInfo{}
	distroDuration := 0 * time.Nanosecond
	distroOverThreshold := 0

	for _, task := range tasks {
		group := task.TaskGroup
		taskGroupID := ""

		if group != "" {
			buildVariant := task.BuildVariant
			project := task.Project
			version := task.Version
			taskGroupID = fmt.Sprintf("%s_%s_%s_%s", group, buildVariant, project, version)
		}

		duration := task.FetchExpectedDuration()
		distroDuration += duration
		overThreshold = (duration > durationThreshold)

		if stats, exists := taskGroupsInfo[taskGroupID]; exists {
			stats.Count++
			stats.ExpectedDuration += duration
			if overThreshold {
				stats.CountOverThreshold++
				stats.DurationOverThreshold++
				distroOverThreshold++
			}
			taskGroupsInfo[taskGroupID] = stats
		} else {
			info := TaskGroupInfo{
				GroupID:          taskGroupID,
				Count:            1,
				ExpectedDuration: duration,
				MaxHosts:         task.TaskGroupMaxHosts,
			}
			if overThreshold {
				info.CountOverThreshold = 1
				info.DurationOverThreshold = duration
				distroOverThreshold = 1
			}
			taskGroupsInfo[taskGroupID] = info
		}
	}

	distroQueueInfo := DistroQueueInfo{
		Distro:             distro,
		Length:             len(runnableTasks),
		ExpectedDuration:   distroDuration,
		CountOverThreshold: distroOverThreshold,
		TaskGroupsInfo:     taskGroupsInfo,
	}

	allocatorData.DistroQueueInfo = distroQueueInfo

	allocator := GetHostAllocator(conf.HostAllocator)
	newHosts, err := allocator(ctx, allocatorData)

	if err != nil {
		return errors.Wrap(err, "problem finding distro")
	}

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
		"operation":     "runtime-stats",
		"phase":         "host-allocation",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startHostAllocation).Seconds(),
	})

	startHostSpawning := time.Now()
	hostsSpawned, err := spawnHosts(ctx, distroc, newHosts, pool)
	if err != nil {
		return errors.Wrap(err, "Error spawning new hosts")
	}

	event.LogSchedulerEvent(event.SchedulerEventData{
		TaskQueueInfo: res.schedulerEvent,
		DistroId:      distro.Id,
	})

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.IdD,
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
		"distro":                 distro.Id,
		"provider":               distro.Provider,
		"max_hosts":              distro.PoolSize,
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

	if d.Id == "" {
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
