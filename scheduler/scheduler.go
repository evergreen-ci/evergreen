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

	// Currently, only the tunable planner is available, so we don't need to check the settings.
	return runTunablePlanner(ctx, d, tasks, opts)
}

func runTunablePlanner(ctx context.Context, d *distro.Distro, tasks []task.Task, opts TaskPlannerOptions) ([]task.Task, error) {
	var err error

	tasks, err = PopulateCaches(ctx, opts.ID, tasks)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	plan := PrepareTasksForPlanning(ctx, d, tasks).Export(ctx)
	info := GetDistroQueueInfo(ctx, d.Id, plan, d.GetTargetTime(), opts)
	info.SecondaryQueue = opts.IsSecondaryQueue
	info.PlanCreatedAt = opts.StartedAt
	if err = PersistTaskQueue(ctx, d.Id, plan, info); err != nil {
		return nil, errors.WithStack(err)
	}

	return plan, nil
}

const RunnerName = "scheduler"

// GetDistroQueueInfo returns the distroQueueInfo for the given set of tasks having set the task.ExpectedDuration for each task.
func GetDistroQueueInfo(ctx context.Context, distroID string, tasks []task.Task, maxDurationThreshold time.Duration, opts TaskPlannerOptions) model.DistroQueueInfo {
	var distroExpectedDuration, distroDurationOverThreshold time.Duration
	var distroCountDurationOverThreshold, distroCountWaitOverThreshold, numTasksDepsMet, numMergeQueueTasks int
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

		duration := task.FetchExpectedDuration(ctx).Average

		if task.DistroId != distroID {
			isSecondaryQueue = true
		}

		var exists bool
		var info *model.TaskGroupInfo
		if info, exists = taskGroupInfosMap[name]; exists {
			if !opts.IncludesDependencies || checkDependenciesMet(ctx, &task, depCache) {
				info.Count++
				info.ExpectedDuration += duration
			}
		} else {
			info = &model.TaskGroupInfo{
				Name:     name,
				MaxHosts: task.TaskGroupMaxHosts,
			}

			if !opts.IncludesDependencies || checkDependenciesMet(ctx, &task, depCache) {
				info.Count++
				info.ExpectedDuration += duration
			}
		}

		dependenciesMet := checkDependenciesMet(ctx, &task, depCache)
		if dependenciesMet {
			numTasksDepsMet++
			if evergreen.IsGithubMergeQueueRequester(task.Requester) {
				numMergeQueueTasks++
				info.CountDepFilledMergeQueueTasks++
			}
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
		Length:                        len(tasks),
		LengthWithDependenciesMet:     numTasksDepsMet,
		ExpectedDuration:              distroExpectedDuration,
		MaxDurationThreshold:          maxDurationThreshold,
		CountDepFilledMergeQueueTasks: numMergeQueueTasks,
		CountDurationOverThreshold:    distroCountDurationOverThreshold,
		DurationOverThreshold:         distroDurationOverThreshold,
		CountWaitOverThreshold:        distroCountWaitOverThreshold,
		TaskGroupInfos:                taskGroupInfos,
		SecondaryQueue:                isSecondaryQueue,
	}

	return distroQueueInfo
}

func checkDependenciesMet(ctx context.Context, t *task.Task, cache map[string]task.Task) bool {
	met, err := t.DependenciesMet(ctx, cache)
	if err != nil {
		return false
	}
	return met

}

// SpawnHosts calls out to the embedded Manager to spawn hosts, and takes in a map of
// distro -> number of hosts to spawn for the distro. It returns a map of distro -> hosts spawned.
// The pool parameter is assumed to be the one from the distro passed in.
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
			intent := generateIntentHost(d)
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
	event.LogManyHostsCreated(ctx, hostIDs)

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
func generateIntentHost(d distro.Distro) *host.Host {
	hostOptions := host.CreateOptions{
		Distro:   d,
		UserName: evergreen.User,
	}
	return host.NewIntent(hostOptions)
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
			if modifiedTask.IsPartOfDisplay(ctx) {
				if err = model.UpdateDisplayTaskForTask(ctx, &modifiedTask); err != nil {
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
