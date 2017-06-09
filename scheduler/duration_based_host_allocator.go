package scheduler

import (
	"math"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/cloud/providers/static"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// maximum turnaround we want to maintain for all hosts for a given distro
	MaxDurationPerDistroHost = time.Hour

	// for distro queues with tasks that appear on other queues, this constant
	// indicates the fraction of the total duration of shared tasks that we want
	// to account for when alternate distros are unable to satisfy the
	// turnaround requirement as determined by MaxDurationPerDistroHost
	SharedTasksAllocationProportion = 0.8
)

// DistroScheduleData contains bookkeeping data that is used by distros to
// determine whether or not to allocate more hosts
type DistroScheduleData struct {

	// indicates the total number of existing hosts for this distro
	numExistingHosts int

	// indicates the nominal number of new hosts to spin up for this distro
	nominalNumNewHosts int

	// indicates the maximum number of hosts allowed for this distro
	poolSize int

	// indicates the number of tasks in this distro's queue
	taskQueueLength int

	// indicates the number of free hosts this distro current has
	numFreeHosts int

	// indicates the total number of seconds (based on the expected running
	// duration) that tasks within a queue (but also appear on other queues)
	// will take to run. It is a map of distro name -> cumulative expected
	// duration of task queue items this distro shares with the keyed distro -
	// the duration is specified in seconds
	sharedTasksDuration map[string]float64

	// indicates the total number of seconds (based on the expected running
	// duration) for tasks currently running on hosts of this distro
	runningTasksDuration float64

	// indicates the total number of seconds (based on the expected running
	// duration) for all tasks currently running on hosts of this distro in
	// addition to the total expected duration of all scheduled tasks on it
	totalTasksDuration float64
}

// ScheduledDistroTasksData contains data that is used to compute the expected
// duration of tasks within a distro's queue
type ScheduledDistroTasksData struct {

	// all tasks in this distro's queue
	taskQueueItems []model.TaskQueueItem

	// all tasks that have been previously accounted for in other distros
	tasksAccountedFor map[string]bool

	// all distros this task could run on
	taskRunDistros map[string][]string

	// the name of the distro whose task queue items data this represents
	currentDistroId string
}

// DurationBasedHostAllocator computes the total time to completion of tasks
// running - per distro - and then uses that as a heuristic in determining
// how many new hosts to spin up
type DurationBasedHostAllocator struct{}

// helper type to sort distros by the number of static hosts they have
type sortableDistroByNumStaticHost struct {
	distros  []distro.Distro
	settings *evergreen.Settings
}

// NewHostsNeeded decides if new hosts are needed for a
// distro while taking the duration of running/scheduled tasks into
// consideration. Returns a map of distro to number of hosts to spawn.
func (self *DurationBasedHostAllocator) NewHostsNeeded(
	hostAllocatorData HostAllocatorData, settings *evergreen.Settings) (newHostsNeeded map[string]int, err error) {

	queueDistros := make([]distro.Distro, 0,
		len(hostAllocatorData.taskQueueItems))

	// Sanity check to ensure that we have a distro object for each item in the
	// task queue. Also pulls the distros we need for sorting
	for distroId := range hostAllocatorData.taskQueueItems {
		distro, ok := hostAllocatorData.distros[distroId]
		if !ok {
			return nil, errors.Errorf("No distro info available for distro %v",
				distroId)
		}
		if distro.Id != distroId {
			return nil, errors.Errorf("Bad mapping between task queue distro "+
				"name and host allocator distro data: %v != %v", distro.Id,
				distroId)
		}
		queueDistros = append(queueDistros, distro)
	}

	// sort the distros by the number of static hosts available. why?
	// well if we have tasks that can run on say 2 distros, one with static
	// hosts and other without, we want to spin up new machines for the latter
	// only if the former is unable to satisfy the turnaround requirement - as
	// determined by MaxDurationPerDistroHost
	distros := sortDistrosByNumStaticHosts(queueDistros, settings)

	// for all distros, this maintains a mapping of distro name -> the number
	// of new hosts needed for that distro
	newHostsNeeded = make(map[string]int)

	// across all distros, this maintains a mapping of task id -> bool - a
	// boolean that indicates if we've accounted for this task from some
	// distro's queue
	tasksAccountedFor := make(map[string]bool)

	// for each distro, this contains data on pertinent information that was
	// used in creating a nominal number of new hosts needed for that distro
	distroScheduleData := make(map[string]DistroScheduleData)

	// now, for each distro, see if we need to spin up any new hosts
	for _, d := range distros {
		newHostsNeeded[d.Id], err = self.
			numNewHostsForDistro(&hostAllocatorData, d, tasksAccountedFor,
				distroScheduleData, settings)
		if err != nil {
			grip.Errorln("Error getting num hosts for distro:", err)
			return nil, err
		}
	}

	grip.InfoWhenf(len(newHostsNeeded) > 0, "Reporting hosts needed: %+v", newHostsNeeded)
	grip.InfoWhen(len(newHostsNeeded) == 0, "no new hosts needed.")

	return newHostsNeeded, nil
}

// computeScheduledTasksDuration returns the total estimated duration of all
// tasks scheduled to be run in a given task queue
func computeScheduledTasksDuration(
	scheduledDistroTasksData *ScheduledDistroTasksData) (
	scheduledTasksDuration float64, sharedTasksDuration map[string]float64) {

	taskQueueItems := scheduledDistroTasksData.taskQueueItems
	taskRunDistros := scheduledDistroTasksData.taskRunDistros
	tasksAccountedFor := scheduledDistroTasksData.tasksAccountedFor
	currentDistroId := scheduledDistroTasksData.currentDistroId
	sharedTasksDuration = make(map[string]float64)

	// compute the total expected duration for tasks in this queue
	for _, taskQueueItem := range taskQueueItems {
		if !tasksAccountedFor[taskQueueItem.Id] {
			scheduledTasksDuration += taskQueueItem.ExpectedDuration.Seconds()
			tasksAccountedFor[taskQueueItem.Id] = true
		}

		// if the task can be run on multiple distros - including this one - add
		// it to the total duration of 'shared tasks' for the distro and all
		// other distros it can be run on
		distroIds, ok := taskRunDistros[taskQueueItem.Id]
		if ok && util.SliceContains(distroIds, currentDistroId) {
			for _, distroId := range distroIds {
				sharedTasksDuration[distroId] +=
					taskQueueItem.ExpectedDuration.Seconds()
			}
		}
	}
	return
}

// computeRunningTasksDuration returns the estimated time to completion of all
// currently running tasks for a given distro given its hosts
func computeRunningTasksDuration(existingDistroHosts []host.Host,
	taskDurations model.ProjectTaskDurations) (runningTasksDuration float64,
	err error) {

	runningTaskIds := []string{}

	for _, existingDistroHost := range existingDistroHosts {
		if existingDistroHost.RunningTask != "" {
			runningTaskIds = append(runningTaskIds,
				existingDistroHost.RunningTask)
		}
	}

	// if this distro's hosts are all free, return immediately
	if len(runningTaskIds) == 0 {
		return
	}

	runningTasksMap := make(map[string]task.Task)
	runningTasks, err := task.Find(task.ByIds(runningTaskIds))
	if err != nil {
		return runningTasksDuration, err
	}

	// build a map of task id => task
	for _, runningTask := range runningTasks {
		runningTasksMap[runningTask.Id] = runningTask
	}

	// compute the total time to completion for running tasks
	for _, runningTaskId := range runningTaskIds {
		runningTask, ok := runningTasksMap[runningTaskId]
		if !ok {
			return runningTasksDuration, errors.Errorf("Unable to find running "+
				"task with _id %v", runningTaskId)
		}
		expectedDuration := model.GetTaskExpectedDuration(runningTask,
			taskDurations)
		elapsedTime := time.Since(runningTask.StartTime)
		if elapsedTime > expectedDuration {
			// probably an outlier; or an unknown data point
			continue
		}
		runningTasksDuration += expectedDuration.Seconds() -
			elapsedTime.Seconds()
	}
	return
}

// computeDurationBasedNumNewHosts returns the number of new hosts needed based
// on a heuristic that utilizes the total duration of currently running and
// scheduled tasks - and based on a maximum duration of a task per distro host -
// a turnaround cap on all outstanding and running tasks in the system
func computeDurationBasedNumNewHosts(scheduledTasksDuration,
	runningTasksDuration, numExistingDistroHosts float64,
	maxDurationPerHost time.Duration) (numNewHosts int) {

	// total duration of scheduled and currently running tasks
	totalDistroTasksDuration := scheduledTasksDuration +
		runningTasksDuration

	// number of hosts needed to meet the duration based turnaround requirement
	numHostsForTurnaroundRequirement := totalDistroTasksDuration /
		maxDurationPerHost.Seconds()

	// floating point precision number of new hosts needed
	durationBasedNumNewHostsNeeded := numHostsForTurnaroundRequirement -
		numExistingDistroHosts

	// duration based number of new hosts needed
	numNewHosts = int(math.Ceil(durationBasedNumNewHostsNeeded))

	// return 0 if numNewHosts is less than 0
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return
}

// fetchExcessSharedDuration returns a slice of duration times (as seconds)
// given a distro and a map of DistroScheduleData, it traverses the map looking
// for alternate distros that are unable to meet maxDurationPerHost (turnaround)
// specification for shared tasks
func fetchExcessSharedDuration(distroScheduleData map[string]DistroScheduleData,
	distro string,
	maxDurationPerHost time.Duration) (sharedTasksDurationTimes []float64) {

	distroData := distroScheduleData[distro]

	// if we have more tasks to run than we have existing hosts and at least one
	// other alternate distro can not run all its shared scheduled and running tasks
	// within the maxDurationPerHost period, we need some more hosts for this distro
	for sharedDistro, sharedDuration := range distroData.sharedTasksDuration {
		if distro == sharedDistro {
			continue
		}

		alternateDistroScheduleData := distroScheduleData[sharedDistro]

		durationPerHost := alternateDistroScheduleData.totalTasksDuration /
			(float64(alternateDistroScheduleData.numExistingHosts) +
				float64(alternateDistroScheduleData.nominalNumNewHosts))

		// if no other alternate distro is able to meet the host requirements,
		// append its shared tasks duration time to the returned slice
		if durationPerHost > maxDurationPerHost.Seconds() {
			sharedTasksDurationTimes = append(sharedTasksDurationTimes,
				sharedDuration)
		}
	}
	return
}

// orderedScheduleNumNewHosts returns the final number of new hosts to spin up
// for this distro. It uses the distroScheduleData map to determine if tasks
// already accounted - tasks that appear in this distro's queue but also were
// accounted for since they appeared in an earlier distro's queue - are too much
// for other distro hosts to handle. In particular, it considers the nominal
// number of new hosts needed (based on the duration estimate) - and then
// revises this number if needed. This is necessary since we use a predefined
// distro order when we determine scheduling (while maintaining a map of all
// queue items we've seen) and allow for a task to run on one or more distros.
// Essentially, it is meant to mitigate the pathological situation where no new
// hosts are spun up for a latter processed distro (even when needed) as items
// on the latter distro's queues are considered already 'accounted for'.
func orderedScheduleNumNewHosts(
	distroScheduleData map[string]DistroScheduleData,
	distro string, maxDurationPerHost time.Duration,
	sharedTasksAllocationProportion float64) int {

	// examine the current distro's schedule data
	distroData := distroScheduleData[distro]

	// if we're spinning up additional new hosts then we don't need any new
	// hosts here so we return the nominal number of new hosts
	if distroData.nominalNumNewHosts != 0 {
		return distroData.nominalNumNewHosts
	}

	// if the current distro does not share tasks with any other distro,
	// return 0
	if distroData.sharedTasksDuration == nil {
		return 0
	}

	// if the current distro can not spin up any more hosts, return 0
	if distroData.poolSize <= distroData.numExistingHosts {
		return 0
	}

	// for distros that share task queue items with this distro, find if any of
	// the distros is unable to satisfy the turnaround requirement as determined
	// by maxDurationPerHost
	sharedTasksDurationTimes := fetchExcessSharedDuration(distroScheduleData,
		distro, maxDurationPerHost)

	// if all alternate distros can meet the turnaround requirements, return 0
	if len(sharedTasksDurationTimes) == 0 {
		return 0
	}

	// if we get here, then it means we need more hosts and don't have any new
	// or pending hosts to handle outstanding tasks for this distro - within the
	// maxDurationPerHost threshold
	sort.Float64s(sharedTasksDurationTimes)

	// we are most interested with the alternate distro with which we have the
	// largest sum total of shared tasks duration
	sharedTasksDuration := sharedTasksDurationTimes[len(
		sharedTasksDurationTimes)-1]

	// we utilize a percentage of the total duration of all 'shared tasks' we
	// want to incorporate in the revised duration based number of new hosts
	// estimate calculation. this percentage is specified by
	// sharedTasksAllocationProportion.
	// Note that this is a subset of the duration of the number of scheduled
	// tasks - specifically, one that only considers shared tasks and ignores
	// tasks that have been exclusively scheduled on this distro alone
	scheduledTasksDuration := distroData.runningTasksDuration +
		sharedTasksDuration

	// in order to conserve money, we take only a fraction of the shared
	// duration in computing the total shared duration
	scheduledTasksDuration *= sharedTasksAllocationProportion

	// make a second call to compute the number of new hosts needed with a
	// revised duration of scheduled tasks
	durationBasedNumNewHosts := computeDurationBasedNumNewHosts(
		scheduledTasksDuration, distroData.runningTasksDuration,
		float64(distroData.numExistingHosts), maxDurationPerHost)

	return numNewDistroHosts(distroData.poolSize, distroData.numExistingHosts,
		distroData.numFreeHosts, durationBasedNumNewHosts,
		distroData.taskQueueLength)
}

// numNewDistroHosts computes the number of new hosts needed as allowed by
// poolSize. if the duration based estimate (durNewHosts) is too large, e.g.
// when there's a small number of very long running tasks, utilize the deficit
// of available hosts vs. tasks to be run
func numNewDistroHosts(poolSize, numExistingHosts, numFreeHosts, durNewHosts,
	taskQueueLength int) (numNewHosts int) {

	numNewHosts = util.Min(
		// the maximum number of new hosts we're allowed to spin up
		poolSize-numExistingHosts,

		// the duration based estimate for the number of new hosts needed
		durNewHosts,

		// the deficit of available hosts vs. tasks to be run
		taskQueueLength-numFreeHosts,
	)

	// cap to zero as lower bound
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return
}

// numNewHostsForDistro determine how many new hosts should be spun up for an
// individual distro.
func (self *DurationBasedHostAllocator) numNewHostsForDistro(
	hostAllocatorData *HostAllocatorData, distro distro.Distro,
	tasksAccountedFor map[string]bool,
	distroScheduleData map[string]DistroScheduleData, settings *evergreen.Settings) (numNewHosts int,
	err error) {

	projectTaskDurations := hostAllocatorData.projectTaskDurations
	existingDistroHosts := hostAllocatorData.existingDistroHosts[distro.Id]
	taskQueueItems := hostAllocatorData.taskQueueItems[distro.Id]
	taskRunDistros := hostAllocatorData.taskRunDistros

	// determine how many free hosts we have
	numFreeHosts := 0
	for _, existingDistroHost := range existingDistroHosts {
		if existingDistroHost.RunningTask == "" {
			numFreeHosts += 1
		}
	}

	// determine the total remaining running time of all
	// tasks currently running on the hosts for this distro
	runningTasksDuration, err := computeRunningTasksDuration(
		existingDistroHosts, projectTaskDurations)

	if err != nil {
		return numNewHosts, err
	}

	// construct the data needed by computeScheduledTasksDuration
	scheduledDistroTasksData := &ScheduledDistroTasksData{
		taskQueueItems:    taskQueueItems,
		tasksAccountedFor: tasksAccountedFor,
		taskRunDistros:    taskRunDistros,
		currentDistroId:   distro.Id,
	}

	// determine the total expected running time of all scheduled
	// tasks for this distro
	scheduledTasksDuration, sharedTasksDuration :=
		computeScheduledTasksDuration(scheduledDistroTasksData)

	// find the number of new hosts needed based on the total estimated
	// duration for all outstanding and in-flight tasks for this distro
	durationBasedNumNewHosts := computeDurationBasedNumNewHosts(
		scheduledTasksDuration, runningTasksDuration,
		float64(len(existingDistroHosts)), MaxDurationPerDistroHost)

	// revise the new host estimate based on the cap of the number of new hosts
	// and the number of free hosts
	numNewHosts = numNewDistroHosts(distro.PoolSize, len(existingDistroHosts),
		numFreeHosts, durationBasedNumNewHosts, len(taskQueueItems))

	// create an entry for this distro in the scheduling map
	distroScheduleData[distro.Id] = DistroScheduleData{
		nominalNumNewHosts:   numNewHosts,
		numFreeHosts:         numFreeHosts,
		poolSize:             distro.PoolSize,
		taskQueueLength:      len(taskQueueItems),
		sharedTasksDuration:  sharedTasksDuration,
		runningTasksDuration: runningTasksDuration,
		numExistingHosts:     len(existingDistroHosts),
		totalTasksDuration:   scheduledTasksDuration + runningTasksDuration,
	}

	cloudManager, err := providers.GetCloudManager(distro.Provider, settings)
	if err != nil {
		err = errors.Wrapf(err, "Couldn't get cloud manager for %s (%s)",
			distro.Provider, distro.Id)
		grip.Error(err)
		return 0, err
	}

	can, err := cloudManager.CanSpawn()
	if err != nil {
		err = errors.Wrapf(err, "Problem checking if '%v' provider can spawn hosts",
			distro.Provider)
		grip.Error(err)
		return 0, nil
	}
	if !can {
		return 0, nil
	}

	// revise the nominal number of new hosts if needed
	numNewHosts = orderedScheduleNumNewHosts(distroScheduleData, distro.Id,
		MaxDurationPerDistroHost, SharedTasksAllocationProportion)

	grip.Infof("Spawning %d additional hosts for %s - currently at %d existing hosts (%d free)",
		numNewHosts, distro.Id, len(existingDistroHosts), numFreeHosts)
	grip.Infof("Total estimated time to process all '%s' scheduled tasks is %s; %d running "+
		"tasks at %s, %d pending tasks at %s (shared tasks duration map: %v)",
		distro.Id,
		time.Duration(scheduledTasksDuration+runningTasksDuration)*time.Second,
		len(existingDistroHosts)-numFreeHosts,
		time.Duration(runningTasksDuration)*time.Second,
		len(taskQueueItems),
		time.Duration(scheduledTasksDuration)*time.Second,
		sharedTasksDuration)

	return numNewHosts, nil
}

// sortDistrosByNumStaticHosts returns a sorted slice of distros where the
// distro with the greatest number of static host is first - at index position 0
func sortDistrosByNumStaticHosts(distros []distro.Distro, settings *evergreen.Settings) []distro.Distro {
	sortableDistroObj := &sortableDistroByNumStaticHost{distros, settings}
	sort.Sort(sortableDistroObj)
	return sortableDistroObj.distros
}

// helpers for sorting the distros by the number of their static hosts
func (sd *sortableDistroByNumStaticHost) Len() int {
	return len(sd.distros)
}

func (sd *sortableDistroByNumStaticHost) Less(i, j int) bool {
	if sd.distros[i].Provider != evergreen.HostTypeStatic &&
		sd.distros[j].Provider != evergreen.HostTypeStatic {
		return false
	}
	if sd.distros[i].Provider == evergreen.HostTypeStatic &&
		sd.distros[j].Provider != evergreen.HostTypeStatic {
		return true
	}
	if sd.distros[i].Provider != evergreen.HostTypeStatic &&
		sd.distros[j].Provider == evergreen.HostTypeStatic {
		return false
	}

	h1 := &static.Settings{}
	h2 := &static.Settings{}

	err := mapstructure.Decode(sd.distros[i].ProviderSettings, h1)
	if err != nil {
		return false
	}

	err = mapstructure.Decode(sd.distros[j].ProviderSettings, h2)
	if err != nil {
		return false
	}

	return len(h1.Hosts) > len(h2.Hosts)
}

func (sd *sortableDistroByNumStaticHost) Swap(i, j int) {
	sd.distros[i], sd.distros[j] =
		sd.distros[j], sd.distros[i]
}
