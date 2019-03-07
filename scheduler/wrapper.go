package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	// maximum turnaround we want to maintain for all hosts for a given distro
	MaxDurationPerDistroHost               = 30 * time.Minute
	MaxDurationPerDistroHostWithContainers = 2 * time.Minute
	dynamicDistroRuntimeAlertThreshold     = 24 * time.Hour
)

func GetDistroQueueInfo(tasks []task.Task, maxDurationThreshold time.Duration) model.DistroQueueInfo {
	var distroExpectedDuration time.Duration
	var distroCountOverThreshold int
	taskGroupInfosMap := make(map[string]model.TaskGroupInfo)

	for _, task := range tasks {
		group := task.TaskGroup
		name := ""
		if group != "" {
			name = task.GetTaskGroupString()
		}

		duration := task.FetchExpectedDuration()
		distroExpectedDuration += duration

		var taskGroupInfo model.TaskGroupInfo
		if info, exists := taskGroupInfosMap[name]; exists {
			info.Count++
			info.ExpectedDuration += duration
			taskGroupInfo = info
		} else {
			taskGroupInfo = model.TaskGroupInfo{
				Name:             name,
				Count:            1,
				MaxHosts:         task.TaskGroupMaxHosts,
				ExpectedDuration: duration,
			}
		}
		if duration >= maxDurationThreshold {
			taskGroupInfo.CountOverThreshold++
			taskGroupInfo.DurationOverThreshold += duration
			distroCountOverThreshold++
		}
		taskGroupInfosMap[name] = taskGroupInfo
	}

	taskGroupInfos := make([]model.TaskGroupInfo, 0, len(taskGroupInfosMap))
	for _, info := range taskGroupInfosMap {
		taskGroupInfos = append(taskGroupInfos, info)
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:             len(tasks),
		ExpectedDuration:   distroExpectedDuration,
		CountOverThreshold: distroCountOverThreshold,
		TaskGroupInfos:     taskGroupInfos,
	}

	return distroQueueInfo
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
		return errors.Wrap(err, "error while filtering tasks against the versions' cache")
	}

	ds := &distroScheduler{
		TaskPrioritizer: &CmpBasedTaskPrioritizer{
			runtimeID: schedulerInstance,
		},
		TaskQueuePersister: &DBTaskQueuePersister{},
		runtimeID:          schedulerInstance,
	}

	startPlanPhase := time.Now()
	prioritizedTasks, err := ds.scheduleDistro(distro.Id, runnableTasks, versions)
	if err != nil {
		return errors.Wrap(err, "problem calculating distro plan")
	}

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
		"operation":     "runtime-stats",
		"phase":         "planning-distro",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startPlanPhase).Seconds(),
		"stat":          "distro-queue-size",
		"size":          len(prioritizedTasks),
	})

	startHostAllocation := time.Now()
	if err = host.RemoveStaleInitializing(distro.Id); err != nil {
		return errors.Wrap(err, "problem removing previously intented hosts, before creating new ones.") // nolint:misspell
	}

	distroHosts, err := host.AllRunningHosts(distro.Id)
	if err != nil {
		return errors.Wrap(err, "with host query")
	}

	hostAllocatorData := HostAllocatorData{
		Distro:               distro,
		ExistingHosts:        distroHosts,
		FreeHostFraction:     conf.FreeHostFraction,
		MaxDurationThreshold: MaxDurationPerDistroHost,
	}

	// retrieve container pool information for container distros
	var pool *evergreen.ContainerPool
	if distro.ContainerPool != "" {
		pool = s.ContainerPools.GetContainerPool(distro.ContainerPool)
		if pool == nil {
			return errors.Wrap(err, "problem retrieving container pool")
		}
		hostAllocatorData.UsesContainers = true
		hostAllocatorData.ContainerPool = pool
		hostAllocatorData.MaxDurationThreshold = MaxDurationPerDistroHostWithContainers
	}

	distroQueueInfo := GetDistroQueueInfo(prioritizedTasks, hostAllocatorData.MaxDurationThreshold)
	hostAllocatorData.DistroQueueInfo = distroQueueInfo

	allocator := GetHostAllocator(conf.HostAllocator)
	newHosts, err := allocator(ctx, hostAllocatorData)
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
	hostsSpawned, err := spawnHosts(ctx, distro, newHosts, pool)
	if err != nil {
		return errors.Wrap(err, "Error spawning new hosts")
	}

	eventInfo := event.TaskQueueInfo{
		TaskQueueLength:  len(prioritizedTasks),
		NumHostsRunning:  0,
		ExpectedDuration: distroQueueInfo.ExpectedDuration,
	}

	event.LogSchedulerEvent(event.SchedulerEventData{
		TaskQueueInfo: eventInfo,
		DistroId:      distro.Id,
	})

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        distro.Id,
		"operation":     "runtime-stats",
		"phase":         "host-spawning",
		"instance":      schedulerInstance,
		"duration_secs": time.Since(startHostSpawning).Seconds(),
	})

	var makespan time.Duration
	if len(hostsSpawned) != 0 {
		makespan = distroQueueInfo.ExpectedDuration / time.Duration(len(hostsSpawned))
	} else if distroQueueInfo.Length > 0 {
		makespan = distroQueueInfo.ExpectedDuration
	}

	grip.Info(message.Fields{
		"message":                "distro-scheduler-report",
		"runner":                 RunnerName,
		"distro":                 distro.Id,
		"provider":               distro.Provider,
		"max_hosts":              distro.PoolSize,
		"new_hosts":              hostsSpawned,
		"num_hosts":              len(hostsSpawned),
		"queue":                  eventInfo,
		"total_runtime":          distroQueueInfo.ExpectedDuration.String(),
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
