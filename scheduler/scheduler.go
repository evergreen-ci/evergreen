package scheduler

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/anser/bsonutil"
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
	RunnerName = "scheduler"

	underwaterPruningEnabled = true
)

type distroSchedulerResult struct {
	distroId       string
	schedulerEvent event.TaskQueueInfo
	taskQueueItem  []model.TaskQueueItem
	err            error
}

type distroSchedueler struct {
	TaskPrioritizer
	TaskQueuePersister
}

func (s *distroSchedueler) scheduleDistro(distroId string, runnableTasksForDistro []task.Task, versions map[string]version.Version) distroSchedulerResult {
	res := distroSchedulerResult{
		distroId: distroId,
	}
	grip.Info(message.Fields{
		"runner":    RunnerName,
		"distro":    distroId,
		"num_tasks": len(runnableTasksForDistro),
	})

	prioritizedTasks, err := s.PrioritizeTasks(distroId, runnableTasksForDistro, versions)
	if err != nil {
		res.err = errors.Wrap(err, "Error prioritizing tasks")
		return res
	}

	// persist the queue of tasks
	grip.Debug(message.Fields{
		"runner":    RunnerName,
		"distro":    distroId,
		"operation": "saving task queue for distro",
	})

	queuedTasks, err := s.PersistTaskQueue(distroId, prioritizedTasks)
	if err != nil {
		res.err = errors.Wrapf(err, "Error processing distro %s saving task queue", distroId)
		return res
	}

	// track scheduled time for prioritized tasks
	err = task.SetTasksScheduledTime(prioritizedTasks, time.Now())
	if err != nil {
		res.err = errors.Wrapf(err,
			"Error processing distro %s setting scheduled time for prioritized tasks",
			distroId)
		return res
	}
	res.taskQueueItem = queuedTasks

	var totalDuration time.Duration
	for _, item := range queuedTasks {
		totalDuration += item.ExpectedDuration
	}
	// initialize the task queue info
	res.schedulerEvent = event.TaskQueueInfo{
		TaskQueueLength:  len(queuedTasks),
		NumHostsRunning:  0,
		ExpectedDuration: totalDuration,
	}

	// final sanity check
	if len(runnableTasksForDistro) != len(res.taskQueueItem) {
		delta := make(map[string]string)
		for _, t := range res.taskQueueItem {
			delta[t.Id] = "res.taskQueueItem"
		}
		for _, i := range runnableTasksForDistro {
			if delta[i.Id] == "res.taskQueueItem" {
				delete(delta, i.Id)
			} else {
				delta[i.Id] = "d.runnableTasksForDistro"
			}
		}
		grip.Alert(message.Fields{
			"runner":             RunnerName,
			"distro":             distroId,
			"message":            "inconsistency with scheduler input and output",
			"inconsistent_tasks": delta,
		})
	}

	return res
}

// Call out to the embedded Manager to spawn hosts.  Takes in a map of
// distro -> number of hosts to spawn for the distro.
// Returns a map of distro -> hosts spawned, and an error if one occurs.
func spawnHosts(ctx context.Context, d distro.Distro, newHostsNeeded int, pool *evergreen.ContainerPool) ([]host.Host, error) {

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

		// find all uninitialized parent intent documents
		numUninitializedParents, err := host.CountUninitializedParents()
		if err != nil {
			return nil, errors.Wrap(err, "could not count uninitialized parents")
		}
		// compute number of parents needed
		numNewParents := numNewParentsNeeded(len(currentParents), numUninitializedParents, newHostsNeeded, len(existingContainers), pool.MaxContainers)

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

	// host.create intent documents for non-parent hosts
	for i := 0; i < newHostsNeeded; i++ {
		intent, err := getIntentHost(d)
		if err != nil {
			return nil, err
		}

		hostsSpawned = append(hostsSpawned, *intent)

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

// generateHostOptions generates host options based on what kind of host it is:
// regular host or container
func generateHostOptions(d distro.Distro) (cloud.HostOptions, error) {
	if d.ContainerPool != "" {
		parent, err := findAvailableParent(d)
		if err != nil {
			err = errors.Wrap(err, "Could not find available parent host")
			return cloud.HostOptions{}, err
		}
		hostOptions := cloud.HostOptions{
			ParentID: parent.Id,
			UserName: evergreen.User,
		}
		return hostOptions, nil
	}

	hostOptions := cloud.HostOptions{
		UserName: evergreen.User,
	}
	return hostOptions, nil
}

// generateParentHostOptions generates host options for a parent host
func generateParentHostOptions(pool *evergreen.ContainerPool) cloud.HostOptions {
	return cloud.HostOptions{
		HasContainers:         true,
		UserName:              evergreen.User,
		ContainerPoolSettings: pool,
	}
}

// FindAvailableParent finds a parent host that can accommodate container,
// packing on parent that has task with longest expected finish time
func findAvailableParent(d distro.Distro) (host.Host, error) {
	allParents, err := host.FindAllRunningParentsByContainerPool(d.ContainerPool)
	if err != nil {
		return host.Host{}, errors.Wrap(err, "Could not find running parent hosts")
	}

	// parents come in sorted order from soonest to latest expected finish time
	for i := len(allParents) - 1; i >= 0; i-- {
		parent := allParents[i]
		currentContainers, err := parent.GetContainers()
		if err != nil {
			return host.Host{}, errors.Wrapf(err, "Could not find containers for parent %s", parent.Id)
		}
		if len(currentContainers) < parent.ContainerPoolSettings.MaxContainers {
			return parent, nil
		}
	}
	return host.Host{}, errors.New("No available parent found for container")
}

// numNewParentsNeeded returns the number of additional parents needed to
// accommodate new containers
func numNewParentsNeeded(numCurrentParents, numUninitializedParents, numContainersNeeded, numExistingContainers, maxContainers int) int {
	if numCurrentParents*maxContainers < numExistingContainers+numContainersNeeded {
		// subtract numUninitializedParents because they will soon come up as parents we can use
		numTotalNewParents := int(math.Ceil(float64(numContainersNeeded)/float64(maxContainers))) - numUninitializedParents
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
	if numContainersToSpawn > 0 {
		numAvailableContainers := numCurrentParents*maxContainers - numCurrentContainers
		if numContainersToSpawn > numAvailableContainers {
			numContainersToSpawn = numAvailableContainers
		}
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

// insertIntent creates a host intent document for a regular host or container
func getIntentHost(d distro.Distro) (*host.Host, error) {
	hostOptions, err := generateHostOptions(d)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not generate host options for distro %s", d.Id)
	}

	return cloud.NewIntent(d, d.GenerateName(), d.Provider, hostOptions), nil
}

// Finds live hosts in the DB and organizes them by distro. Pass the
// empty string to retrieve all distros
func findUsableHosts(distroID string) ([]host.Host, error) {
	// fetch all hosts, split by distro
	query := host.IsLive()
	if distroID != "" {
		key := bsonutil.GetDottedKeyName(host.DistroKey, distro.IdKey)
		query[key] = distroID
	}

	allHosts, err := host.Find(db.Query(query))
	if err != nil {
		return nil, errors.Wrap(err, "Error finding live hosts")
	}

	return allHosts, nil
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
