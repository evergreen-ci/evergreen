package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// Responsible for prioritizing and scheduling tasks to be run, on a per-distro
// basis.
type Scheduler struct {
	*evergreen.Settings
	TaskPrioritizer
	TaskQueuePersister
	HostAllocator

	FindRunnableTasks TaskFinder
}

const (
	RunnerName               = "scheduler"
	underwaterPruningEnabled = true
)

type distroScheduler struct {
	runtimeID string
	TaskPrioritizer
	TaskQueuePersister
}

func (s *distroScheduler) scheduleDistro(distroID string, runnableTasksForDistro []task.Task, versions map[string]model.Version, maxDurationThreshold time.Duration) (
	[]task.Task, error) {

	grip.Info(message.Fields{
		"runner":    RunnerName,
		"distro":    distroID,
		"num_tasks": len(runnableTasksForDistro),
		"instance":  s.runtimeID,
	})

	prioritizedTasks, err := s.PrioritizeTasks(distroID, runnableTasksForDistro, versions)
	if err != nil {
		return nil, errors.Wrapf(err, "error prioritizing tasks for distro '%s'", distroID)

	}

	distroQueueInfo := GetDistroQueueInfo(prioritizedTasks, maxDurationThreshold)

	grip.Debug(message.Fields{
		"runner":    RunnerName,
		"distro":    distroID,
		"instance":  s.runtimeID,
		"operation": "saving task queue for distro",
	})

	// persist the queue of tasks and its associated distroQueueInfo
	persistTime := time.Now() // Get time before persisting tasks so that scheduled time is never after start time.
	_, err = s.PersistTaskQueue(distroID, prioritizedTasks, distroQueueInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "database error saving the task queue for distro '%s'", distroID)
	}

	// track scheduled time for prioritized tasks
	err = task.SetTasksScheduledTime(prioritizedTasks, persistTime)
	if err != nil {
		return nil, errors.Wrapf(err, "error setting scheduled time for prioritized tasks for distro '%s'", distroID)
	}

	return prioritizedTasks, nil
}

// Returns the distroQueueInfo for the given set of tasks having set the task.ExpectedDuration for each task.
func GetDistroQueueInfo(tasks []task.Task, maxDurationThreshold time.Duration) model.DistroQueueInfo {
	var distroExpectedDuration time.Duration
	var distroCountOverThreshold int
	taskGroupInfosMap := make(map[string]model.TaskGroupInfo)

	for i, task := range tasks {
		group := task.TaskGroup
		name := ""
		if group != "" {
			name = task.GetTaskGroupString()
		}

		duration := task.FetchExpectedDuration()
		task.ExpectedDuration = duration
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
		tasks[i] = task
	}

	taskGroupInfos := make([]model.TaskGroupInfo, 0, len(taskGroupInfosMap))
	for _, info := range taskGroupInfosMap {
		taskGroupInfos = append(taskGroupInfos, info)
	}

	distroQueueInfo := model.DistroQueueInfo{
		Length:               len(tasks),
		ExpectedDuration:     distroExpectedDuration,
		MaxDurationThreshold: maxDurationThreshold,
		CountOverThreshold:   distroCountOverThreshold,
		TaskGroupInfos:       taskGroupInfos,
	}

	return distroQueueInfo
}

// Call out to the embedded Manager to spawn hosts.  Takes in a map of
// distro -> number of hosts to spawn for the distro.
// Returns a map of distro -> hosts spawned, and an error if one occurs.
func SpawnHosts(ctx context.Context, d distro.Distro, newHostsNeeded int, pool *evergreen.ContainerPool) ([]host.Host, error) {
	startTime := time.Now()

	if newHostsNeeded == 0 {
		return []host.Host{}, nil
	}
	numHostsToSpawn := newHostsNeeded
	hostsSpawned := []host.Host{}
	distroStartTime := time.Now()

	if ctx.Err() != nil {
		return nil, errors.New("scheduling run canceled")
	}

	// if distro is container distro, check if there are enough parent hosts to support new containers
	var newParentHosts []host.Host
	if pool != nil {
		var err error
		// only want to spawn amount of parents allowed based on pool size
		newParentHosts, numHostsToSpawn, err = host.InsertParentIntentsAndGetNumHostsToSpawn(pool, newHostsNeeded, false)
		if err != nil {
			return nil, errors.Wrap(err, "could not generate new parents hosts needed")
		}
		if len(newParentHosts) > 0 {
			grip.Info(message.Fields{
				"runner":          RunnerName,
				"distro":          d.Id,
				"pool":            pool.Id,
				"pool_distro":     pool.Distro,
				"num_new_parents": len(newParentHosts),
				"operation":       "spawning new parents",
				"duration_secs":   time.Since(distroStartTime).Seconds(),
			})
		}
	}

	// create intent documents for container hosts
	if d.ContainerPool != "" {
		hostOptions, err := getCreateOptionsFromDistro(d)
		if err != nil {
			return nil, errors.Wrapf(err, "Error getting docker options from distro %s", d.Id)
		}
		containerIntents, err := host.GenerateContainerHostIntents(d, numHostsToSpawn, *hostOptions)
		if err != nil {
			return nil, errors.Wrap(err, "error generating container intent hosts")
		}
		hostsSpawned = append(hostsSpawned, containerIntents...)
	} else { // create intent documents for regular hosts
		for i := 0; i < numHostsToSpawn; i++ {
			intent, err := generateIntentHost(d)
			if err != nil {
				return nil, errors.Wrap(err, "error generating intent host")
			}
			hostsSpawned = append(hostsSpawned, *intent)
		}
	}

	if err := host.InsertMany(hostsSpawned); err != nil {
		return nil, errors.Wrap(err, "problem inserting host documents")
	}

	grip.Info(message.Fields{
		"runner":        RunnerName,
		"distro":        d.Id,
		"operation":     "spawning instances",
		"duration_secs": time.Since(startTime).Seconds(),
		"num_hosts":     len(hostsSpawned),
	})
	hostsSpawned = append(newParentHosts, hostsSpawned...)
	return hostsSpawned, nil
}

func getCreateOptionsFromDistro(d distro.Distro) (*host.CreateOptions, error) {
	dockerOptions, err := getDockerOptionsFromProviderSettings(*d.ProviderSettings)
	if err != nil {
		return nil, errors.Wrapf(err, "Error getting docker options from distro %s", d.Id)
	}
	hostOptions := host.CreateOptions{
		UserName:      evergreen.User,
		DockerOptions: *dockerOptions,
	}
	return &hostOptions, nil
}

func getDockerOptionsFromProviderSettings(settings map[string]interface{}) (*host.DockerOptions, error) {
	dockerOptions := &host.DockerOptions{}
	if settings != nil {
		if err := mapstructure.Decode(settings, dockerOptions); err != nil {
			return nil, errors.Wrap(err, "Error decoding params")
		}
	}
	if dockerOptions.Image == "" {
		return nil, errors.New("docker image cannot be empty")
	}
	return dockerOptions, nil
}

// generateIntentHost creates a host intent document for a regular host
func generateIntentHost(d distro.Distro) (*host.Host, error) {
	hostOptions := host.CreateOptions{
		UserName: evergreen.User,
	}
	return host.NewIntent(d, d.GenerateName(), d.Provider, hostOptions), nil
}

// pass 'allDistros' or the empty string to unchedule all distros.
func underwaterUnschedule(distroID string) error {
	if underwaterPruningEnabled {
		num, err := task.UnscheduleStaleUnderwaterTasks(distroID)
		if err != nil {
			return errors.WithStack(err)
		}

		grip.InfoWhen(num > 0, message.Fields{
			"message": "unscheduled stale tasks",
			"runner":  RunnerName,
			"count":   num,
		})
	}

	return nil
}
