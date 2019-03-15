package scheduler

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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

type newParentsNeededParams struct {
	numUphostParents, numContainersNeeded, numExistingContainers, maxContainers int
}

type containersOnParents struct {
	parentHost    host.Host
	numContainers int
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
		return nil, errors.Wrap(err, "Error prioritizing tasks")
	}

	distroQueueInfo := GetDistroQueueInfo(prioritizedTasks, maxDurationThreshold)

	grip.Debug(message.Fields{
		"runner":    RunnerName,
		"distro":    distroID,
		"instance":  s.runtimeID,
		"operation": "saving task queue for distro",
	})

	// persist the queue of tasks and its associated distroQueueInfo
	_, err = s.PersistTaskQueue(distroID, prioritizedTasks, distroQueueInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "Error processing distro %s saving task queue", distroID)
	}

	// track scheduled time for prioritized tasks
	err = task.SetTasksScheduledTime(prioritizedTasks, time.Now())
	if err != nil {
		return nil, errors.Wrapf(err, "Error setting scheduled time for prioritized tasks for distro '%s'", distroID)
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

	// loop over the distros, spawning up the appropriate number of hosts
	// for each distro

	hostsSpawned := []host.Host{}

	distroStartTime := time.Now()

	if ctx.Err() != nil {
		return nil, errors.New("scheduling run canceled")
	}

	// if distro is container distro, check if there are enough parent hosts to
	// support new containers
	if pool != nil {
		// find all running parents with the specified container pool
		currentParents, err := host.FindAllRunningParentsByContainerPool(pool.Id)
		if err != nil {
			return nil, errors.Wrap(err, "could not find running parents")
		}

		// find all child containers running on those parents
		existingContainers, err := host.HostGroup(currentParents).FindRunningContainersOnParents()
		if err != nil {
			return nil, errors.Wrap(err, "could not find running containers")
		}

		// find all uphost parent intent documents
		numUphostParents, err := host.CountUphostParentsByContainerPool(pool.Id)
		if err != nil {
			return nil, errors.Wrap(err, "could not count uphost parents")
		}

		// create numParentsNeededParams struct
		parentsParams := newParentsNeededParams{
			numUphostParents:      numUphostParents,
			numContainersNeeded:   newHostsNeeded,
			numExistingContainers: len(existingContainers),
			maxContainers:         pool.MaxContainers,
		}
		// compute number of parents needed
		numNewParents := numNewParentsNeeded(parentsParams)

		// get parent distro from pool
		parentDistro, err := distro.FindOne(distro.ById(pool.Distro))
		if err != nil {
			return nil, errors.Wrap(err, "error find parent distro")
		}

		// only want to spawn amount of parents allowed based on pool size
		numNewParentsToSpawn, err := parentCapacity(parentDistro, numNewParents, len(currentParents), pool)
		if err != nil {
			return nil, errors.Wrap(err, "could not calculate number of parents needed to spawn")
		}
		// create parent host intent documents
		if numNewParentsToSpawn > 0 {
			hostsSpawned = append(hostsSpawned, createParents(parentDistro, numNewParentsToSpawn, pool)...)

			grip.Info(message.Fields{
				"runner":          RunnerName,
				"distro":          d.Id,
				"pool":            pool.Id,
				"pool_distro":     pool.Distro,
				"num_new_parents": numNewParentsToSpawn,
				"operation":       "spawning new parents",
				"duration_secs":   time.Since(distroStartTime).Seconds(),
			})
		}

		// only want to spawn amount of containers we can fit on currently running parents
		newHostsNeeded = containerCapacity(len(currentParents), len(existingContainers), newHostsNeeded, pool.MaxContainers)
	}

	// create intent documents for container hosts
	if d.ContainerPool != "" {
		containerIntents, err := generateContainerHostIntents(d, newHostsNeeded)
		if err != nil {
			return nil, errors.Wrap(err, "error generating container intent hosts")
		}
		hostsSpawned = append(hostsSpawned, containerIntents...)
	} else { // create intent documents for regular hosts
		for i := 0; i < newHostsNeeded; i++ {
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

	return hostsSpawned, nil
}

// generateContainerHostIntents generates container intent documents by going
// through available parents and packing on the parents with longest expected
// finish time
func generateContainerHostIntents(d distro.Distro, newContainersNeeded int) ([]host.Host, error) {
	parents, err := getNumContainersOnParents(d)
	if err != nil {
		err = errors.Wrap(err, "Could not find number of containers on each parent")
		return nil, err
	}
	containerHostIntents := make([]host.Host, 0)
	for _, parent := range parents {
		// find out how many more containers this parent can fit
		containerSpace := parent.parentHost.ContainerPoolSettings.MaxContainers - parent.numContainers
		containersToCreate := containerSpace
		// only create containers as many as we need
		if newContainersNeeded < containerSpace {
			containersToCreate = newContainersNeeded
		}
		for i := 0; i < containersToCreate; i++ {
			hostOptions := cloud.HostOptions{
				ParentID: parent.parentHost.Id,
				UserName: evergreen.User,
			}
			containerHostIntents = append(containerHostIntents, *cloud.NewIntent(d, d.GenerateName(), d.Provider, hostOptions))
		}
		newContainersNeeded -= containersToCreate
		if newContainersNeeded == 0 {
			return containerHostIntents, nil
		}
	}
	return containerHostIntents, nil
}

// generateParentHostOptions generates host options for a parent host
func generateParentHostOptions(pool *evergreen.ContainerPool) cloud.HostOptions {
	return cloud.HostOptions{
		HasContainers:         true,
		UserName:              evergreen.User,
		ContainerPoolSettings: pool,
	}
}

// getNumContainersOnParents returns a slice of parents and their respective
// number of current containers currently running in order of longest expected
// finish time
func getNumContainersOnParents(d distro.Distro) ([]containersOnParents, error) {
	allParents, err := host.FindAllRunningParentsByContainerPool(d.ContainerPool)
	if err != nil {
		return nil, errors.Wrap(err, "Could not find running parent hosts")
	}

	numContainersOnParents := make([]containersOnParents, 0)
	// parents come in sorted order from soonest to latest expected finish time
	for i := len(allParents) - 1; i >= 0; i-- {
		parent := allParents[i]
		currentContainers, err := parent.GetContainers()
		if err != nil {
			return nil, errors.Wrapf(err, "Could not find containers for parent %s", parent.Id)
		}
		if len(currentContainers) < parent.ContainerPoolSettings.MaxContainers {
			numContainersOnParents = append(numContainersOnParents,
				containersOnParents{
					parentHost:    parent,
					numContainers: len(currentContainers),
				})
		}
	}
	return numContainersOnParents, nil
}

// numNewParentsNeeded returns the number of additional parents needed to
// accommodate new containers
func numNewParentsNeeded(params newParentsNeededParams) int {
	if params.numUphostParents*params.maxContainers < params.numExistingContainers+params.numContainersNeeded {
		numTotalNewParents := int(math.Ceil(float64(params.numContainersNeeded) / float64(params.maxContainers)))
		if numTotalNewParents < 0 {
			return 0
		}
		return numTotalNewParents
	}
	return 0
}

// parentCapacity calculates number of new parents to create
// checks to make sure we do not create more parents than allowed
func parentCapacity(parent distro.Distro, numNewParents, numCurrentParents int, pool *evergreen.ContainerPool) (int, error) {
	if parent.Provider == evergreen.ProviderNameStatic {
		return 0, nil
	}
	// if there are already maximum numbers of parents running, do not spawn
	// any more parents
	if numCurrentParents >= parent.PoolSize {
		numNewParents = 0
	}
	// if adding all new parents results in more parents than allowed, only add
	// enough parents to fill to capacity
	if numNewParents+numCurrentParents > parent.PoolSize {
		numNewParents = parent.PoolSize - numCurrentParents
	}
	return numNewParents, nil
}

// containerCapacity calculates how many containers to make
// checks to make sure we do not create more containers than can fit currently
func containerCapacity(numCurrentParents, numCurrentContainers, numContainersToSpawn, maxContainers int) int {
	if numContainersToSpawn < 0 {
		return 0
	}
	numAvailableContainers := numCurrentParents*maxContainers - numCurrentContainers
	if numContainersToSpawn > numAvailableContainers {
		return numAvailableContainers
	}
	return numContainersToSpawn
}

// createParents creates host intent documents for each parent
func createParents(parent distro.Distro, numNewParents int, pool *evergreen.ContainerPool) []host.Host {
	hostsSpawned := make([]host.Host, numNewParents)

	for idx := range hostsSpawned {
		hostsSpawned[idx] = *cloud.NewIntent(parent, parent.GenerateName(), parent.Provider, generateParentHostOptions(pool))
	}

	return hostsSpawned
}

// generateIntentHost creates a host intent document for a regular host
func generateIntentHost(d distro.Distro) (*host.Host, error) {
	hostOptions := cloud.HostOptions{
		UserName: evergreen.User,
	}
	return cloud.NewIntent(d, d.GenerateName(), d.Provider, hostOptions), nil
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
