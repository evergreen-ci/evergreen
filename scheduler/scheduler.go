package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type TaskPlannerOptions struct {
	ID                   string
	IsSecondaryQueue     bool
	IncludesDependencies bool
	StartedAt            time.Time
}

type TaskPlanner func(*distro.Distro, []task.Task, TaskPlannerOptions) ([]task.Task, error)

func PrioritizeTasks(ctx context.Context, d *distro.Distro, tasks []task.Task, opts TaskPlannerOptions) ([]task.Task, error) {
	opts.IncludesDependencies = d.DispatcherSettings.Version == evergreen.DispatcherVersionRevisedWithDependencies

	switch d.PlannerSettings.Version {
	case evergreen.PlannerVersionTunable:
		return runTunablePlanner(ctx, d, tasks, opts)
	default:
		return runLegacyPlanner(d, tasks, opts)
	}
}

func runTunablePlanner(ctx context.Context, d *distro.Distro, tasks []task.Task, opts TaskPlannerOptions) ([]task.Task, error) {
	var err error

	tasks, err = PopulateCaches(opts.ID, tasks)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	plan := PrepareTasksForPlanning(d, tasks).Export(ctx)
	info := GetDistroQueueInfo(d.Id, plan, d.GetTargetTime(), opts)
	info.SecondaryQueue = opts.IsSecondaryQueue
	info.PlanCreatedAt = opts.StartedAt
	if err = PersistTaskQueue(d.Id, plan, info); err != nil {
		return nil, errors.WithStack(err)
	}

	return plan, nil
}

////////////////////////////////////////////////////////////////////////
//
// UseLegacy Scheduler Implementation

func runLegacyPlanner(d *distro.Distro, tasks []task.Task, opts TaskPlannerOptions) ([]task.Task, error) {
	runnableTasks, versions, err := FilterTasksWithVersionCache(tasks)
	if err != nil {
		return nil, errors.Wrap(err, "filtering tasks against the versions' cache")
	}

	ds := &distroScheduler{
		TaskPrioritizer: &CmpBasedTaskPrioritizer{
			runtimeID: opts.ID,
		},
		opts:      opts,
		runtimeID: opts.ID,
		startedAt: opts.StartedAt,
	}

	prioritizedTasks, err := ds.scheduleDistro(d.Id, runnableTasks, versions, d.GetTargetTime(), opts.IsSecondaryQueue)
	if err != nil {
		return nil, errors.Wrapf(err, "calculating distro plan for distro '%s'", d.Id)
	}

	return prioritizedTasks, nil
}

// Responsible for prioritizing and scheduling tasks to be run, on a per-distro
// basis.
type Scheduler struct {
	*evergreen.Settings
	TaskPrioritizer
	HostAllocator

	FindRunnableTasks TaskFinder
}

const RunnerName = "scheduler"

type distroScheduler struct {
	startedAt time.Time
	runtimeID string
	opts      TaskPlannerOptions
	TaskPrioritizer
}

func (s *distroScheduler) scheduleDistro(distroID string, runnableTasks []task.Task, versions map[string]model.Version, maxThreshold time.Duration, isSecondaryQueue bool) ([]task.Task, error) {
	prioritizedTasks, _, err := s.PrioritizeTasks(distroID, runnableTasks, versions)
	if err != nil {
		return nil, errors.Wrapf(err, "prioritizing tasks for distro '%s'", distroID)

	}

	distroQueueInfo := GetDistroQueueInfo(distroID, prioritizedTasks, maxThreshold, s.opts)
	distroQueueInfo.SecondaryQueue = isSecondaryQueue
	distroQueueInfo.PlanCreatedAt = s.startedAt

	// persist the queue of tasks and its associated distroQueueInfo
	err = PersistTaskQueue(distroID, prioritizedTasks, distroQueueInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "saving the task queue for distro '%s'", distroID)
	}

	return prioritizedTasks, nil
}

// GetDistroQueueInfo returns the distroQueueInfo for the given set of tasks having set the task.ExpectedDuration for each task.
func GetDistroQueueInfo(distroID string, tasks []task.Task, maxDurationThreshold time.Duration, opts TaskPlannerOptions) model.DistroQueueInfo {
	var distroExpectedDuration, distroDurationOverThreshold time.Duration
	var distroCountDurationOverThreshold, distroCountWaitOverThreshold, numTasksDepsMet int
	var isSecondaryQueue bool
	taskGroupInfosMap := make(map[string]*model.TaskGroupInfo)
	depCache := make(map[string]task.Task, len(tasks))
	for _, t := range tasks {
		depCache[t.Id] = t
	}

	for i, task := range tasks {
		group := task.TaskGroup
		name := ""
		if group != "" {
			name = task.GetTaskGroupString()
		}

		duration := task.FetchExpectedDuration().Average

		if task.DistroId != distroID {
			isSecondaryQueue = true
		}

		var exists bool
		var info *model.TaskGroupInfo
		if info, exists = taskGroupInfosMap[name]; exists {
			if !opts.IncludesDependencies || checkDependenciesMet(&task, depCache) {
				info.Count++
				info.ExpectedDuration += duration
			}
		} else {
			info = &model.TaskGroupInfo{
				Name:     name,
				MaxHosts: task.TaskGroupMaxHosts,
			}

			if !opts.IncludesDependencies || checkDependenciesMet(&task, depCache) {
				info.Count++
				info.ExpectedDuration += duration
			}
		}

		dependenciesMet := checkDependenciesMet(&task, depCache)
		if dependenciesMet {
			numTasksDepsMet++
		}
		if !opts.IncludesDependencies || dependenciesMet {
			task.ExpectedDuration = duration
			distroExpectedDuration += duration
			// duration is defined as expected runtime and does not include wait time
			if duration > maxDurationThreshold {
				if info != nil {
					info.CountDurationOverThreshold++
					info.DurationOverThreshold += duration
				}
				distroCountDurationOverThreshold++
				distroDurationOverThreshold += duration
			}
			if dependenciesMet {
				startTime := task.ScheduledTime
				if task.DependenciesMetTime.After(startTime) {
					startTime = task.DependenciesMetTime
				}
				task.WaitSinceDependenciesMet = time.Since(startTime)

				// actual wait time allows us to independently check that the threshold is working
				if task.WaitSinceDependenciesMet > maxDurationThreshold {
					if info != nil {
						info.CountWaitOverThreshold++
					}
					distroCountWaitOverThreshold++
				}
			}

		}

		taskGroupInfosMap[name] = info
		tasks[i] = task
	}

	taskGroupInfos := make([]model.TaskGroupInfo, 0, len(taskGroupInfosMap))
	for tgName := range taskGroupInfosMap {
		taskGroupInfos = append(taskGroupInfos, *taskGroupInfosMap[tgName])
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:                     len(tasks),
		LengthWithDependenciesMet:  numTasksDepsMet,
		ExpectedDuration:           distroExpectedDuration,
		MaxDurationThreshold:       maxDurationThreshold,
		CountDurationOverThreshold: distroCountDurationOverThreshold,
		DurationOverThreshold:      distroDurationOverThreshold,
		CountWaitOverThreshold:     distroCountWaitOverThreshold,
		TaskGroupInfos:             taskGroupInfos,
		SecondaryQueue:             isSecondaryQueue,
	}

	return distroQueueInfo
}

func checkDependenciesMet(t *task.Task, cache map[string]task.Task) bool {
	met, err := t.DependenciesMet(cache)
	if err != nil {
		return false
	}
	return met

}

// Call out to the embedded Manager to spawn hosts.  Takes in a map of
// distro -> number of hosts to spawn for the distro.
// Returns a map of distro -> hosts spawned, and an error if one occurs.
// The pool parameter is assumed to be the one from the distro passed in
func SpawnHosts(ctx context.Context, d distro.Distro, newHostsNeeded int, pool *evergreen.ContainerPool) ([]host.Host, error) {
	startTime := time.Now()

	if newHostsNeeded == 0 {
		return []host.Host{}, nil
	}
	numHostsToSpawn := newHostsNeeded
	hostsSpawned := []host.Host{}

	if ctx.Err() != nil {
		return nil, errors.New("scheduling run canceled")
	}

	// if distro is container distro, check if there are enough parent hosts to support new containers
	if pool != nil {
		hostOptions, err := getCreateOptionsFromDistro(d)
		if err != nil {
			return nil, errors.Wrapf(err, "getting Docker options from distro '%s'", d.Id)
		}
		newContainers, newParents, err := host.MakeContainersAndParents(ctx, d, pool, newHostsNeeded, *hostOptions)
		if err != nil {
			return nil, errors.Wrapf(err, "creating container intents for distro '%s'", d.Id)
		}
		hostsSpawned = append(hostsSpawned, newContainers...)
		hostsSpawned = append(hostsSpawned, newParents...)
		grip.Info(message.Fields{
			"runner":             RunnerName,
			"distro":             d.Id,
			"pool":               pool.Id,
			"pool_distro":        pool.Distro,
			"num_new_parents":    len(newParents),
			"num_new_containers": len(newContainers),
			"operation":          "spawning new parents",
			"duration_secs":      time.Since(startTime).Seconds(),
		})
	} else { // create intent documents for regular hosts
		for i := 0; i < numHostsToSpawn; i++ {
			intent, err := generateIntentHost(d, pool)
			if err != nil {
				return nil, errors.Wrap(err, "generating intent host")
			}
			hostsSpawned = append(hostsSpawned, *intent)
		}
	}

	if err := host.InsertMany(ctx, hostsSpawned); err != nil {
		return nil, errors.Wrap(err, "inserting intent host documents")
	}

	hostIDs := make([]string, 0, len(hostsSpawned))
	for _, h := range hostsSpawned {
		hostIDs = append(hostIDs, h.Id)
	}
	event.LogManyHostsCreated(hostIDs)

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        d.Id,
		"operation":     "spawning instances",
		"duration_secs": time.Since(startTime).Seconds(),
		"num_hosts":     len(hostsSpawned),
	})
	return hostsSpawned, nil
}

func getCreateOptionsFromDistro(d distro.Distro) (*host.CreateOptions, error) {
	dockerOptions := &host.DockerOptions{}
	if err := dockerOptions.FromDistroSettings(d, ""); err != nil {
		return nil, errors.Wrapf(err, "getting Docker options from distro '%s'", d.Id)
	}
	if err := dockerOptions.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	hostOptions := host.CreateOptions{
		Distro:        d,
		UserName:      evergreen.User,
		DockerOptions: *dockerOptions,
	}
	return &hostOptions, nil
}

// generateIntentHost creates a host intent document for a regular host
func generateIntentHost(d distro.Distro, pool *evergreen.ContainerPool) (*host.Host, error) {
	hostOptions := host.CreateOptions{
		Distro:   d,
		UserName: evergreen.User,
	}
	if pool != nil {
		hostOptions.ContainerPoolSettings = pool
		hostOptions.HasContainers = true
	}
	return host.NewIntent(hostOptions), nil
}

// pass the empty string to unschedule all distros.
func underwaterUnschedule(ctx context.Context, distroID string) error {
	modifiedTasks, err := task.UnscheduleStaleUnderwaterHostTasks(ctx, distroID)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(modifiedTasks) > 0 {
		taskIDs := make([]string, 0, len(modifiedTasks))
		for _, modifiedTask := range modifiedTasks {
			taskIDs = append(taskIDs, modifiedTask.Id)
			if err = model.UpdateBuildAndVersionStatusForTask(ctx, &modifiedTask); err != nil {
				return errors.Wrapf(err, "updating build and version status for task '%s'", modifiedTask.Id)
			}
			if modifiedTask.IsPartOfDisplay() {
				if err = model.UpdateDisplayTaskForTask(&modifiedTask); err != nil {
					return errors.Wrap(err, "updating parent display task")
				}
			}
		}
		grip.Info(message.Fields{
			"message":  "unscheduled stale tasks",
			"distro":   distroID,
			"runner":   RunnerName,
			"task_ids": taskIDs,
		})
	}

	return nil
}
