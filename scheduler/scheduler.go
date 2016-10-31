package scheduler

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
)

// Responsible for prioritizing and scheduling tasks to be run, on a per-distro
// basis.
type Scheduler struct {
	*evergreen.Settings
	TaskFinder
	TaskPrioritizer
	TaskDurationEstimator
	TaskQueuePersister
	HostAllocator
}

// versionBuildVariant is used to keep track of the version/buildvariant fields
// for tasks that are to be split by distro
type versionBuildVariant struct {
	Version, BuildVariant string
}

// Schedule all of the tasks to be run.  Works by finding all of the tasks that
// are ready to be run, splitting them by distro, prioritizing them, and saving
// the per-distro queues.  Then determines the number of new hosts to spin up
// for each distro, and spins them up.
func (s *Scheduler) Schedule() error {
	// make sure the correct static hosts are in the database
	evergreen.Logger.Logf(slogger.INFO, "Updating static hosts...")

	err := model.UpdateStaticHosts(s.Settings)
	if err != nil {
		return fmt.Errorf("error updating static hosts: %v", err)
	}

	// find all tasks ready to be run
	evergreen.Logger.Logf(slogger.INFO, "Finding runnable tasks...")

	runnableTasks, err := s.FindRunnableTasks()
	if err != nil {
		return fmt.Errorf("Error finding runnable tasks: %v", err)
	}

	evergreen.Logger.Logf(slogger.INFO, "There are %v tasks ready to be run", len(runnableTasks))

	// split the tasks by distro
	tasksByDistro, taskRunDistros, err := s.splitTasksByDistro(runnableTasks)
	if err != nil {
		return fmt.Errorf("Error splitting tasks by distro to run on: %v", err)
	}

	// load in all of the distros
	distros, err := distro.Find(distro.All)
	if err != nil {
		return fmt.Errorf("Error finding distros: %v", err)
	}

	// get the expected run duration of all runnable tasks
	taskExpectedDuration, err := s.GetExpectedDurations(runnableTasks)

	if err != nil {
		return fmt.Errorf("Error getting expected task durations: %v", err)
	}

	distroInputChan := make(chan distroSchedulerInput, len(distros))

	// put all of the needed input for the distro scheduler into a channel to be read by the
	// distro scheduling loop
	for distroId, task := range tasksByDistro {
		distroInputChan <- distroSchedulerInput{
			distroId:               distroId,
			runnableTasksForDistro: task,
		}
	}
	// close the channel to signal that the loop reading from it can terminate
	close(distroInputChan)
	workers := runtime.NumCPU()

	wg := sync.WaitGroup{}
	wg.Add(workers)

	// make a channel to collect all of function results from scheduling the distros
	distroSchedulerResultChan := make(chan *distroSchedulerResult)

	// for each worker, create a new goroutine
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			// read the inputs for scheduling this distro
			for d := range distroInputChan {
				// schedule the distro
				res := s.scheduleDistro(d.distroId, d.runnableTasksForDistro, taskExpectedDuration)
				if res.err != nil {
					evergreen.Logger.Logf(slogger.ERROR, "%v", err)
				}

				// write the results out to a results channel
				distroSchedulerResultChan <- res

			}
		}()
	}

	// signal the errCollector goroutine that it can terminate it's loop
	// intialize a map of scheduler events
	schedulerEvents := map[string]event.TaskQueueInfo{}

	// prioritize the tasks, one distro at a time
	taskQueueItems := make(map[string][]model.TaskQueueItem)

	var errResult error
	go func() {
		for res := range distroSchedulerResultChan {
			if res.err != nil {
				errResult = fmt.Errorf("error scheduling tasks on distro %v: %v", res.distroId, err)
				return
			}
			schedulerEvents[res.distroId] = res.schedulerEvent
			taskQueueItems[res.distroId] = res.taskQueueItem
		}
	}()

	if errResult != nil {
		return errResult
	}

	// wait for the distro scheduler goroutines to complete to complete
	wg.Wait()
	// wait group has terminated so scheduler channel can be closed
	close(distroSchedulerResultChan)

	// split distros by name
	distrosByName := make(map[string]distro.Distro)
	for _, d := range distros {
		distrosByName[d.Id] = d
	}

	// fetch all hosts, split by distro
	allHosts, err := host.Find(host.IsLive)
	if err != nil {
		return fmt.Errorf("Error finding live hosts: %v", err)
	}

	// figure out all hosts we have up - per distro
	hostsByDistro := make(map[string][]host.Host)
	for _, liveHost := range allHosts {
		hostsByDistro[liveHost.Distro.Id] = append(hostsByDistro[liveHost.Distro.Id],
			liveHost)
	}

	// add the length of the host lists of hosts that are running to the event log.
	for distroId, hosts := range hostsByDistro {
		taskQueueInfo := schedulerEvents[distroId]
		taskQueueInfo.NumHostsRunning = len(hosts)
		schedulerEvents[distroId] = taskQueueInfo
	}

	// construct the data that will be needed by the host allocator
	hostAllocatorData := HostAllocatorData{
		existingDistroHosts:  hostsByDistro,
		distros:              distrosByName,
		taskQueueItems:       taskQueueItems,
		taskRunDistros:       taskRunDistros,
		projectTaskDurations: taskExpectedDuration,
	}

	// figure out how many new hosts we need
	newHostsNeeded, err := s.NewHostsNeeded(hostAllocatorData, s.Settings)
	if err != nil {
		return fmt.Errorf("Error determining how many new hosts are needed: %v",
			err)
	}

	// spawn up the hosts
	hostsSpawned, err := s.spawnHosts(newHostsNeeded)
	if err != nil {
		return fmt.Errorf("Error spawning new hosts: %v", err)
	}

	if len(hostsSpawned) != 0 {
		evergreen.Logger.Logf(slogger.INFO, "Hosts spawned (%v total), by distro: ",
			len(hostsSpawned))
		for distro, hosts := range hostsSpawned {
			evergreen.Logger.Logf(slogger.INFO, "  %v ->", distro)
			for _, host := range hosts {
				evergreen.Logger.Logf(slogger.INFO, "    %v", host.Id)
			}

			taskQueueInfo := schedulerEvents[distro]
			taskQueueInfo.NumHostsRunning += len(hosts)
			schedulerEvents[distro] = taskQueueInfo
		}
	} else {
		evergreen.Logger.Logf(slogger.INFO, "No new hosts spawned")
	}

	for d, t := range schedulerEvents {
		eventLog := event.SchedulerEventData{
			ResourceType:  event.ResourceTypeScheduler,
			TaskQueueInfo: t,
			DistroId:      d,
		}
		event.LogSchedulerEvent(eventLog)
	}

	return nil
}

type distroSchedulerInput struct {
	distroId               string
	runnableTasksForDistro []task.Task
}

type distroSchedulerResult struct {
	distroId       string
	schedulerEvent event.TaskQueueInfo
	taskQueueItem  []model.TaskQueueItem
	err            error
}

func (s *Scheduler) scheduleDistro(distroId string, runnableTasksForDistro []task.Task,
	taskExpectedDuration model.ProjectTaskDurations) *distroSchedulerResult {

	res := distroSchedulerResult{
		distroId: distroId,
	}
	evergreen.Logger.Logf(slogger.INFO, "Prioritizing %v tasks for distro %v...",
		len(runnableTasksForDistro), distroId)

	prioritizedTasks, err := s.PrioritizeTasks(s.Settings,
		runnableTasksForDistro)
	if err != nil {
		res.err = fmt.Errorf("Error prioritizing tasks: %v", err)
		return &res
	}

	// persist the queue of tasks
	evergreen.Logger.Logf(slogger.INFO, "Saving task queue for distro %v...", distroId)
	queuedTasks, err := s.PersistTaskQueue(distroId, prioritizedTasks,
		taskExpectedDuration)
	if err != nil {
		res.err = fmt.Errorf("Error processing distro %v saving task queue: %v", distroId, err)
		return &res
	}

	// track scheduled time for prioritized tasks
	err = task.SetTasksScheduledTime(prioritizedTasks, time.Now())
	if err != nil {
		res.err = fmt.Errorf("Error processing distro %v setting scheduled time for prioritized "+
			"tasks: %v", distroId, err)
		return &res
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
	return &res

}

// Takes in a version id and a map of "key -> buildvariant" (where "key" is of
// type "versionBuildVariant") and updates the map with an entry for the
// buildvariants associated with "versionStr"
func (s *Scheduler) updateVersionBuildVarMap(versionStr string,
	versionBuildVarMap map[versionBuildVariant]model.BuildVariant) (err error) {
	version, err := version.FindOne(version.ById(versionStr))
	if err != nil {
		return
	}
	if version == nil {
		return fmt.Errorf("nil version returned for version id '%v'", versionStr)
	}
	project := &model.Project{}

	err = model.LoadProjectInto([]byte(version.Config), version.Identifier, project)
	if err != nil {
		return fmt.Errorf("unable to load project config for version %v: "+
			"%v", versionStr, err)
	}

	// create buildvariant map (for accessing purposes)
	for _, buildVariant := range project.BuildVariants {
		key := versionBuildVariant{versionStr, buildVariant.Name}
		versionBuildVarMap[key] = buildVariant
	}
	return
}

// Takes in a list of tasks, and splits them by distro.
// Returns a map of distro name -> tasks that can be run on that distro
// and a map of task id -> distros that the task can be run on (for tasks
// that can be run on multiple distro)
func (s *Scheduler) splitTasksByDistro(tasksToSplit []task.Task) (
	map[string][]task.Task, map[string][]string, error) {
	tasksByDistro := make(map[string][]task.Task)
	taskRunDistros := make(map[string][]string)

	// map of versionBuildVariant -> build variant
	versionBuildVarMap := make(map[versionBuildVariant]model.BuildVariant)

	// insert the tasks into the appropriate distro's queue in our map
	for _, task := range tasksToSplit {
		key := versionBuildVariant{task.Version, task.BuildVariant}
		if _, exists := versionBuildVarMap[key]; !exists {
			err := s.updateVersionBuildVarMap(task.Version, versionBuildVarMap)
			if err != nil {
				evergreen.Logger.Logf(slogger.WARN, "error getting buildvariant "+
					"map for task %v: %v", task.Id, err)
				continue
			}
		}

		// get the build variant for the task
		buildVariant, ok := versionBuildVarMap[key]
		if !ok {
			evergreen.Logger.Logf(slogger.WARN, "task %v has no buildvariant called "+
				"'%v' on project %v", task.Id, task.BuildVariant, task.Project)
			continue
		}

		// get the task specification for the build variant
		var taskSpec model.BuildVariantTask
		for _, tSpec := range buildVariant.Tasks {
			if tSpec.Name == task.DisplayName {
				taskSpec = tSpec
				break
			}
		}

		// if no matching spec was found log it and continue
		if taskSpec.Name == "" {
			evergreen.Logger.Logf(slogger.WARN, "task %v has no matching spec for "+
				"build variant %v on project %v", task.Id,
				task.BuildVariant, task.Project)
			continue
		}

		// use the specified distros for the task, or, if none are specified,
		// the default distros for the build variant
		distrosToUse := buildVariant.RunOn
		if len(taskSpec.Distros) != 0 {
			distrosToUse = taskSpec.Distros
		}
		// remove duplicates to avoid scheduling twice
		distrosToUse = util.UniqueStrings(distrosToUse)
		for _, d := range distrosToUse {
			tasksByDistro[d] = append(tasksByDistro[d], task)
		}

		// for tasks that can run on multiple distros, keep track of which
		// distros they will be scheduled on
		if len(distrosToUse) > 1 {
			taskRunDistros[task.Id] = distrosToUse
		}
	}

	return tasksByDistro, taskRunDistros, nil

}

// Call out to the embedded CloudManager to spawn hosts.  Takes in a map of
// distro -> number of hosts to spawn for the distro.
// Returns a map of distro -> hosts spawned, and an error if one occurs.
func (s *Scheduler) spawnHosts(newHostsNeeded map[string]int) (
	map[string][]host.Host, error) {

	// loop over the distros, spawning up the appropriate number of hosts
	// for each distro
	hostsSpawnedPerDistro := make(map[string][]host.Host)
	for distroId, numHostsToSpawn := range newHostsNeeded {

		if numHostsToSpawn == 0 {
			continue
		}

		hostsSpawnedPerDistro[distroId] = make([]host.Host, 0, numHostsToSpawn)
		for i := 0; i < numHostsToSpawn; i++ {
			d, err := distro.FindOne(distro.ById(distroId))
			if err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "Failed to find distro '%v': %v", distroId, err)
			}

			allDistroHosts, err := host.Find(host.ByDistroId(distroId))
			if err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "Error getting hosts for distro %v: %v", distroId, err)
				continue
			}

			if len(allDistroHosts) >= d.PoolSize {
				evergreen.Logger.Logf(slogger.ERROR, "Already at max (%v) hosts for distro '%v'",
					distroId,
					d.PoolSize)
				continue
			}

			cloudManager, err := providers.GetCloudManager(d.Provider, s.Settings)
			if err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error getting cloud manager for distro: %v", err)
				continue
			}

			hostOptions := cloud.HostOptions{
				UserName: evergreen.User,
				UserHost: false,
			}
			newHost, err := cloudManager.SpawnInstance(d, hostOptions)
			if err != nil {
				evergreen.Logger.Errorf(slogger.ERROR, "Error spawning instance: %v,",
					err)
				continue
			}
			hostsSpawnedPerDistro[distroId] =
				append(hostsSpawnedPerDistro[distroId], *newHost)

		}
		// if none were spawned successfully
		if len(hostsSpawnedPerDistro[distroId]) == 0 {
			delete(hostsSpawnedPerDistro, distroId)
		}
	}
	return hostsSpawnedPerDistro, nil
}
