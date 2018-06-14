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
func spawnHosts(ctx context.Context, newHostsNeeded map[string]int) (map[string][]host.Host, error) {
	startTime := time.Now()

	// loop over the distros, spawning up the appropriate number of hosts
	// for each distro
	hostsSpawnedPerDistro := make(map[string][]host.Host)

	for distroId, numHostsToSpawn := range newHostsNeeded {
		distroStartTime := time.Now()

		if numHostsToSpawn == 0 {
			continue
		}

		if ctx.Err() != nil {
			return nil, errors.New("scheduling run canceled.")
		}

		d, err := distro.FindOne(distro.ById(distroId))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to find distro %s", distroId)
		}

		hostsSpawnedPerDistro[distroId] = make([]host.Host, 0, numHostsToSpawn)
		// if distro can have containers, check if there are enough parents to hold
		// new containers
		if d.MaxContainers > 0 {
			currentParents, err := host.FindAllRunningParentsByDistro(distroId)
			if err != nil {
				return nil, errors.Wrap(err, "could not find running parents")
			}
			existingContainers, err := host.FindAllRunningContainers()
			if err != nil {
				return nil, errors.Wrap(err, "could not find running containers")
			}

			numNewParents := numNewParentsNeeded(len(currentParents), numHostsToSpawn,
				len(existingContainers), d)

			// only want to spawn amount of parents allowed based on pool size
			numNewParentsToSpawn := parentCapacity(d, numNewParents, len(currentParents), len(existingContainers), numHostsToSpawn)

			if numNewParentsToSpawn > 0 {
				parentIntentHosts, err := insertParents(d, numNewParentsToSpawn)
				if err != nil {
					return nil, err
				}
				hostsSpawnedPerDistro[distroId] = append(hostsSpawnedPerDistro[distroId], parentIntentHosts...)

				grip.Info(message.Fields{
					"runner":      RunnerName,
					"distro":      distroId,
					"num_parents": numNewParentsToSpawn,
					"operation":   "spawning new parents",
					"span":        time.Since(distroStartTime).String(),
					"duration":    time.Since(distroStartTime),
				})
			}

			// only want to spawn amount of containers we can fit on currently running parents
			numHostsToSpawn = containerCapacity(d, len(currentParents), len(existingContainers), numHostsToSpawn)
		}

		for i := 0; i < numHostsToSpawn; i++ {
			intentHost, err := insertIntent(d)
			if err != nil {
				return nil, err
			}
			hostsSpawnedPerDistro[distroId] = append(hostsSpawnedPerDistro[distroId], *intentHost)

		}
		// if none were spawned successfully
		if len(hostsSpawnedPerDistro[distroId]) == 0 {
			delete(hostsSpawnedPerDistro, distroId)
		}

		grip.Info(message.Fields{
			"runner":    RunnerName,
			"distro":    distroId,
			"number":    numHostsToSpawn,
			"operation": "spawning instances",
			"span":      time.Since(distroStartTime).String(),
			"duration":  time.Since(distroStartTime),
		})
	}

	grip.Info(message.Fields{
		"runner":    RunnerName,
		"operation": "host query and processing",
		"span":      time.Since(startTime).String(),
		"duration":  time.Since(startTime),
	})
	return hostsSpawnedPerDistro, nil
}

// generateHostOptions generates host options based on what kind of host it is:
// regular host or container
func generateHostOptions(d distro.Distro) (cloud.HostOptions, error) {
	if d.MaxContainers > 0 {
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
func generateParentHostOptions() cloud.HostOptions {
	return cloud.HostOptions{
		HasContainers: true,
		UserName:      evergreen.User,
	}
}

// FindAvailableParent finds a parent host that can accommodate container,
// packing on parent that has task with longest expected finish time
func findAvailableParent(d distro.Distro) (host.Host, error) {
	allParents, err := host.FindAllRunningParentsOrdered()
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
		if len(currentContainers) < d.MaxContainers {
			return parent, nil
		}
	}
	return host.Host{}, errors.New("No available parent found for container")
}

// numNewParentsNeeded returns the number of additional parents needed to
// accommodate new containers
func numNewParentsNeeded(numCurrentParents, numContainersNeeded, numExistingContainers int, d distro.Distro) int {
	if numCurrentParents*d.MaxContainers < numExistingContainers+numContainersNeeded {
		return int(math.Ceil(float64(numContainersNeeded) / float64(d.MaxContainers)))
	}
	return 0
}

// parentCapacity calculates number of new parents to create
// checks to make sure we do not create more parents than allowed
func parentCapacity(d distro.Distro, numNewParents, numCurrentParents, numCurrentContainers, numContainersToSpawn int) int {
	// if there are already maximum numbers of parents running, only spawn
	// enough containers to reach maximum capacity
	if numCurrentParents >= d.PoolSize {
		numNewParents = 0
	}
	// if adding all new parents results in more parents than allowed, only add
	// enough parents to fill to capacity
	if numNewParents+numCurrentParents > d.PoolSize {
		numNewParents = d.MaxContainers - numCurrentParents
	}
	return numNewParents
}

// containerCapacity calculates how many containers to make
// checks to make sure we do not create more containers than can fit currently
func containerCapacity(d distro.Distro, numCurrentParents, numCurrentContainers, numContainersToSpawn int) int {
	if numContainersToSpawn > 0 {
		numAvailableContainers := numCurrentParents*d.MaxContainers - numCurrentContainers
		if numContainersToSpawn > numAvailableContainers {
			numContainersToSpawn = numAvailableContainers
		}
	}
	return numContainersToSpawn
}

// insertParents creates host intent documents for each parent
func insertParents(d distro.Distro, numNewParents int) ([]host.Host, error) {
	hostsSpawned := make([]host.Host, 0)
	for j := 0; j < numNewParents; j++ {
		parentHostOptions := generateParentHostOptions()

		intentHost := cloud.NewIntent(d, d.GenerateName(), d.Provider, parentHostOptions)
		if err := intentHost.Insert(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":   RunnerName,
				"host":     intentHost.Id,
				"provider": d.Provider,
			}))
			return nil, errors.Wrapf(err, "Could not insert parent '%s'", intentHost.Id)
		}
		hostsSpawned = append(hostsSpawned, *intentHost)
	}
	return hostsSpawned, nil
}

// insertIntent creates a host intent document for a regular host or container
func insertIntent(d distro.Distro) (*host.Host, error) {
	hostOptions, err := generateHostOptions(d)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not generate host options for distro %s", d.Id)
	}

	intentHost := cloud.NewIntent(d, d.GenerateName(), d.Provider, hostOptions)
	if err := intentHost.Insert(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"distro":   d.Id,
			"runner":   RunnerName,
			"host":     intentHost.Id,
			"provider": d.Provider,
		}))

		return nil, errors.Wrapf(err, "Could not insert intent host '%s'", intentHost.Id)
	}
	return intentHost, nil
}

// Finds live hosts in the DB and organizes them by distro. Pass the
// empty string to retrieve all distros
func findUsableHosts(distroID string) (map[string][]host.Host, error) {
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

	// figure out all hosts we have up - per distro
	hostsByDistro := make(map[string][]host.Host)
	for _, liveHost := range allHosts {
		hostsByDistro[liveHost.Distro.Id] = append(hostsByDistro[liveHost.Distro.Id],
			liveHost)
	}

	return hostsByDistro, nil
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
