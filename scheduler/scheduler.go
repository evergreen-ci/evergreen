package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/model"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"10gen.com/mci/model/version"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"math"
	"time"
)

// Responsible for prioritizing and scheduling tasks to be run, on a per-distro
// basis.
type Scheduler struct {
	*mci.MCISettings
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
func (self *Scheduler) Schedule() error {

	// make sure the correct static hosts are in the database
	mci.Logger.Logf(slogger.INFO, "Updating static hosts...")
	err := model.RefreshStaticHosts(self.ConfigDir)
	if err != nil {
		return fmt.Errorf("error updating static hosts: %v", err)
	}

	// find all tasks ready to be run
	mci.Logger.Logf(slogger.INFO, "Finding runnable tasks...")
	runnableTasks, err := self.FindRunnableTasks()
	if err != nil {
		return fmt.Errorf("Error finding runnable tasks: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "There are %v tasks ready to be run",
		len(runnableTasks))

	// split the tasks by distro
	tasksByDistro, taskRunDistros, err := self.splitTasksByDistro(runnableTasks)
	if err != nil {
		return fmt.Errorf("Error splitting tasks by distro to run on: %v", err)
	}

	// load in all of the distros
	distros, err := distro.Load(self.MCISettings.ConfigDir)
	if err != nil {
		return fmt.Errorf("Error finding distros: %v", err)
	}

	taskIdToMinQueuePos := make(map[string]int)

	// get the expected run duration of all runnable tasks
	taskExpectedDuration, err := self.GetExpectedDurations(runnableTasks)

	if err != nil {
		return fmt.Errorf("Error getting expected task durations: %v", err)
	}

	// prioritize the tasks, one distro at a time
	taskQueueItems := make(map[string][]model.TaskQueueItem)
	for _, distro := range distros {
		runnableTasksForDistro := tasksByDistro[distro.Name]
		mci.Logger.Logf(slogger.INFO, "Prioritizing %v tasks for distro %v...",
			len(runnableTasksForDistro), distro.Name)

		prioritizedTasks, err := self.PrioritizeTasks(self.MCISettings,
			runnableTasksForDistro)
		if err != nil {
			return fmt.Errorf("Error prioritizing tasks: %v", err)
		}

		// Update the running minimums of queue position
		// The value is 1-based primarily so that we can differentiate between
		// no value and being first in a queue
		for i, prioritizedTask := range prioritizedTasks {
			minQueuePos, ok := taskIdToMinQueuePos[prioritizedTask.Id]
			if ok {
				taskIdToMinQueuePos[prioritizedTask.Id] =
					int(math.Min(float64(minQueuePos), float64(i+1)))
			} else {
				taskIdToMinQueuePos[prioritizedTask.Id] = i + 1
			}
		}

		// persist the queue of tasks
		mci.Logger.Logf(slogger.INFO, "Saving task queue for distro %v...",
			distro.Name)
		queuedTasks, err := self.PersistTaskQueue(distro.Name, prioritizedTasks,
			taskExpectedDuration)
		if err != nil {
			return fmt.Errorf("Error saving task queue: %v", err)
		}

		// track scheduled time for prioritized tasks
		err = model.SetTasksScheduledTime(prioritizedTasks, time.Now())
		if err != nil {
			return fmt.Errorf("Error setting scheduled time for prioritized "+
				"tasks: %v", err)
		}

		taskQueueItems[distro.Name] = queuedTasks
	}

	err = model.UpdateMinQueuePos(taskIdToMinQueuePos)
	if err != nil {
		return fmt.Errorf("Error updating tasks with queue positions: %v", err)
	}

	// split distros by name
	distrosByName := make(map[string]distro.Distro)
	for _, distro := range distros {
		distrosByName[distro.Name] = distro
	}

	// fetch all hosts, split by distro
	allHosts, err := host.Find(host.IsLive)
	if err != nil {
		return fmt.Errorf("Error finding live hosts: %v", err)
	}

	// figure out all hosts we have up - per distro
	hostsByDistro := make(map[string][]host.Host)
	for _, liveHost := range allHosts {
		hostsByDistro[liveHost.Distro] = append(hostsByDistro[liveHost.Distro],
			liveHost)
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
	newHostsNeeded, err := self.NewHostsNeeded(hostAllocatorData, self.MCISettings)
	if err != nil {
		return fmt.Errorf("Error determining how many new hosts are needed: %v",
			err)
	}

	// spawn up the hosts
	hostsSpawned, err := self.spawnHosts(newHostsNeeded)
	if err != nil {
		return fmt.Errorf("Error spawning new hosts: %v", err)
	}

	if len(hostsSpawned) != 0 {
		mci.Logger.Logf(slogger.INFO, "Hosts spawned (%v total), by distro: ",
			len(hostsSpawned))
		for distro, hosts := range hostsSpawned {
			mci.Logger.Logf(slogger.INFO, "  %v ->", distro)
			for _, host := range hosts {
				mci.Logger.Logf(slogger.INFO, "    %v", host.Id)
			}
		}
	} else {
		mci.Logger.Logf(slogger.INFO, "No new hosts spawned")
	}

	return nil
}

// Takes in a version id and a map of "key -> buildvariant" (where "key" is of
// type "versionBuildVariant") and updates the map with an entry for the
// buildvariants associated with "versionStr"
func (self *Scheduler) updateVersionBuildVarMap(versionStr string,
	versionBuildVarMap map[versionBuildVariant]model.BuildVariant) (err error) {
	version, err := version.FindOne(version.ById(versionStr))
	if err != nil {
		return
	}
	if version == nil {
		return fmt.Errorf("nil version returned for version id '%v'", versionStr)
	}
	project := &model.Project{}

	err = model.LoadProjectInto([]byte(version.Config), project)
	if err != nil {
		return fmt.Errorf("unable to unmarshal project config for version %v: "+
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
func (self *Scheduler) splitTasksByDistro(tasksToSplit []model.Task) (
	map[string][]model.Task, map[string][]string, error) {
	tasksByDistro := make(map[string][]model.Task)
	taskRunDistros := make(map[string][]string)

	// map of versionBuildVariant -> build variant
	versionBuildVarMap := make(map[versionBuildVariant]model.BuildVariant)

	// insert the tasks into the appropriate distro's queue in our map
	for _, task := range tasksToSplit {
		key := versionBuildVariant{task.Version, task.BuildVariant}
		if _, exists := versionBuildVarMap[key]; !exists {
			err := self.updateVersionBuildVarMap(task.Version, versionBuildVarMap)
			if err != nil {
				mci.Logger.Logf(slogger.WARN, "error getting buildvariant "+
					"map for task %v: %v", task.Id, err)
				continue
			}
		}

		// get the build variant for the task
		buildVariant, ok := versionBuildVarMap[key]
		if !ok {
			mci.Logger.Logf(slogger.WARN, "task %v has no buildvariant called "+
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
			mci.Logger.Logf(slogger.WARN, "task %v has no matching spec for "+
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
		for _, distro := range distrosToUse {
			tasksByDistro[distro] = append(tasksByDistro[distro], task)
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
func (self *Scheduler) spawnHosts(newHostsNeeded map[string]int) (
	map[string][]host.Host, error) {

	// loop over the distros, spawning up the appropriate number of hosts
	// for each distro
	hostsSpawnedPerDistro := make(map[string][]host.Host)
	for distroName, numHostsToSpawn := range newHostsNeeded {

		if numHostsToSpawn == 0 {
			continue
		}

		hostsSpawnedPerDistro[distroName] = make([]host.Host, 0,
			numHostsToSpawn)
		for i := 0; i < numHostsToSpawn; i++ {
			distro, err := distro.LoadOne(self.ConfigDir, distroName)
			if err != nil || distro == nil {
				mci.Logger.Logf(slogger.ERROR, "Failed to find distro '%v': %v", distroName, err)
			}

			allDistroHosts, err := host.Find(host.ByDistroId(distroName))
			if err != nil {
				mci.Logger.Logf(slogger.ERROR, "Error getting hosts for distro %v: %v", distroName, err)
				continue
			}

			if len(allDistroHosts) >= distro.MaxHosts {
				mci.Logger.Logf(slogger.ERROR, "Already at max (%v) hosts for distro '%v'",
					distro.Name,
					distro.MaxHosts)
				continue
			}

			cloudManager, err := providers.GetCloudManager(distro.Provider, self.MCISettings)
			if err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error getting cloud manager for distro: %v", err)
				continue
			}

			newHost, err := cloudManager.SpawnInstance(distro, mci.MCIUser, false)
			if err != nil {
				mci.Logger.Errorf(slogger.ERROR, "Error spawning instance: %v,",
					err)
				continue
			}
			hostsSpawnedPerDistro[distroName] =
				append(hostsSpawnedPerDistro[distroName], *newHost)

		}
		// if none were spawned successfully
		if len(hostsSpawnedPerDistro[distroName]) == 0 {
			delete(hostsSpawnedPerDistro, distroName)
		}
	}
	return hostsSpawnedPerDistro, nil
}
