package scheduler

import (
	"context"
	"log"
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

type NumParentsContainers struct {
	NumParentsNeeded    int
	NumContainersNeeded int
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
			grip.Error(message.WrapError(err, message.Fields{
				"distro":  distroId,
				"runner":  RunnerName,
				"message": "failed to find distro",
			}))
			continue
		}

		CurrentParents, err := host.FindAllRunningParents()
		if err != nil {
			return nil, err
		}
		ExistingContainers, err := host.FindAllRunningContainers()
		if err != nil {
			return nil, err
		}

		hostsSpawnedPerDistro[distroId] = make([]host.Host, 0, numHostsToSpawn)

		// if distro can have containers, check if there are enough parents to hold
		// new containers
		if d.MaxContainers != 0 {
			numNewParents := CalcNewParentsNeeded(len(CurrentParents),
				len(ExistingContainers), numHostsToSpawn, d)

			// if there are already maximum numbers of parents running, only spawn
			// enough containers to reach maximum capacity
			if numNewParents+len(CurrentParents) > d.PoolSize {
				numHostsToSpawn = (len(CurrentParents) * d.MaxContainers) - len(ExistingContainers)
				numNewParents = 0
			}

			// if there are parents to spawn, do not spawn containers in the same run
			if numNewParents > 0 {
				for j := 0; j < numNewParents; j++ {
					ParentHostOptions := GenerateParentHostOptions()

					intentHost := cloud.NewIntent(d, d.GenerateName(), d.Provider, ParentHostOptions)
					if err := intentHost.Insert(); err != nil {
						err = errors.Wrapf(err, "Could not insert parent '%s'", intentHost.Id)

						grip.Error(message.WrapError(err, message.Fields{
							"runner":   RunnerName,
							"host":     intentHost.Id,
							"provider": d.Provider,
						}))
						return nil, err
					}
					intentHost.Status = "ready"
					hostsSpawnedPerDistro[distroId] = append(hostsSpawnedPerDistro[distroId], *intentHost)
				}
				numHostsToSpawn = 0

				grip.Info(message.Fields{
					"runner":    RunnerName,
					"distro":    distroId,
					"number":    numNewParents,
					"operation": "spawning new parents",
					"span":      time.Since(distroStartTime).String(),
					"duration":  time.Since(distroStartTime),
				})
			}

			// if there are containers to spawn, do not spawn parents in the same run
			if numHostsToSpawn > 0 {
				numNewParents = 0
			}

		}

		for i := 0; i < numHostsToSpawn; i++ {
			allDistroHosts, err := host.Find(host.ByDistroId(distroId))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"distro":  distroId,
					"runner":  RunnerName,
					"message": "problem getting hosts for distro",
				}))

				continue
			}

			if IsMaxHostsRunning(d, allDistroHosts) {
				continue
			}

			hostOptions, err := GenerateHostOptions(d)
			if err != nil {
				continue
			}

			intentHost := cloud.NewIntent(d, d.GenerateName(), d.Provider, hostOptions)
			if err := intentHost.Insert(); err != nil {
				err = errors.Wrapf(err, "Could not insert intent host '%s'", intentHost.Id)

				grip.Error(message.WrapError(err, message.Fields{
					"distro":   distroId,
					"runner":   RunnerName,
					"host":     intentHost.Id,
					"provider": d.Provider,
				}))

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

// IsMaxHostsRunning returns true if the max number of hosts are already running
func IsMaxHostsRunning(d distro.Distro, allDistroHosts []host.Host) bool {
	if d.MaxContainers != 0 {
		if len(allDistroHosts) >= (d.PoolSize * d.MaxContainers) {
			grip.Info(message.Fields{
				"distro":         d.Id,
				"runner":         RunnerName,
				"pool_size":      d.PoolSize,
				"max_containers": d.MaxContainers,
				"message":        "max hosts running on all containers",
			})
			return true
		}
	}

	if len(allDistroHosts) >= d.PoolSize {
		grip.Info(message.Fields{
			"distro":    d.Id,
			"runner":    RunnerName,
			"pool_size": d.PoolSize,
			"message":   "max hosts running",
		})
		return true
	}

	return false
}

// GenerateHostOptions generates host options based on what kind of host it is:
// regular host or container
func GenerateHostOptions(d distro.Distro) (cloud.HostOptions, error) {
	if d.MaxContainers != 0 {
		parent, err := FindAvailableParent(d)
		if err != nil {
			return cloud.HostOptions{}, err
		}
		log.Print("id ", parent.Id)
		hostOptions := cloud.HostOptions{
			ParentID: parent.Id,
			UserName: evergreen.User,
			UserHost: false,
		}
		return hostOptions, nil
	}

	hostOptions := cloud.HostOptions{
		UserName: evergreen.User,
		UserHost: false,
	}
	return hostOptions, nil
}

// GenerateParentHostOptions generates host options for a parent host
func GenerateParentHostOptions() cloud.HostOptions {
	return cloud.HostOptions{
		HasContainers: true,
		UserName:      evergreen.User,
		UserHost:      false,
	}
}

// FindAvailableParent finds a parent host that can accomodate container
// based on oldest parent
func FindAvailableParent(d distro.Distro) (host.Host, error) {
	AllParents, err := host.FindAllRunningParents()
	if err != nil {
		return host.Host{}, err
	}

	OldestParent := host.Host{CreationTime: time.Now()}
	for _, parent := range AllParents {
		CurrentContainers, err := parent.GetContainers()
		if err != nil {
			return host.Host{}, err
		}
		if parent.CreationTime.Before(OldestParent.CreationTime) && len(CurrentContainers) < d.MaxContainers {
			OldestParent = parent
		}
	}
	return OldestParent, nil

}

// CalcNewParentsNeeded returns the number of additional parents needed to
// accomodate new containers
func CalcNewParentsNeeded(numCurrentParents, numExistingContainers, numContainersNeeded int, d distro.Distro) int {
	if numCurrentParents*d.MaxContainers <= numExistingContainers+numContainersNeeded {
		return int(math.Ceil(float64(numContainersNeeded) / float64(d.MaxContainers)))
	}
	return 0
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
