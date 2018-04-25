package scheduler

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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

	GetExpectedDurations TaskDurationEstimator
	FindRunnableTasks    TaskFinder
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

func (s *distroSchedueler) scheduleDistro(distroId string, runnableTasksForDistro []task.Task,
	taskExpectedDuration model.ProjectTaskDurations) distroSchedulerResult {

	res := distroSchedulerResult{
		distroId: distroId,
	}
	grip.Info(message.Fields{
		"runner":    RunnerName,
		"distro":    distroId,
		"num_tasks": len(runnableTasksForDistro),
	})

	prioritizedTasks, err := s.PrioritizeTasks(distroId, runnableTasksForDistro)
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

	queuedTasks, err := s.PersistTaskQueue(distroId, prioritizedTasks,
		taskExpectedDuration)
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

// Call out to the embedded CloudManager to spawn hosts.  Takes in a map of
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

		hostsSpawnedPerDistro[distroId] = make([]host.Host, 0, numHostsToSpawn)
		for i := 0; i < numHostsToSpawn; i++ {
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

			allDistroHosts, err := host.Find(host.ByDistroId(distroId))
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"distro":  distroId,
					"runner":  RunnerName,
					"message": "problem getting hosts for distro",
				}))

				continue
			}

			if len(allDistroHosts) >= d.PoolSize {
				grip.Info(message.Fields{
					"distro":    distroId,
					"runner":    RunnerName,
					"pool_size": d.PoolSize,
					"message":   "max hosts running",
				})

				continue
			}

			hostOptions := cloud.HostOptions{
				UserName: evergreen.User,
				UserHost: false,
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
