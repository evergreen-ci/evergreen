package scheduler

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
)

// type taskGroupData struct {
// 	hosts []host.Host
// 	tasks []model.TaskQueueItem
// }

type taskGroupData struct {
	hosts []host.Host
	info  TaskGroupInfo
}

///////////////////////////////////////////////////////////////////////
// newHosts, err := evalHostUtilization(ctx,
// 	distro,
// 	data.tasks,
// 	data.hosts,
// 	hostAllocatorData.freeHostFraction,
// 	hostAllocatorData.usesContainers,
// 	hostAllocatorData.containerPool,
// 	maxHosts)
///////////////////////////////////////////////////////////////////////

func evalHostUtilization(ctx context.Context, d distro.Distro, taskQueue []model.TaskQueueItem, existingHosts []host.Host,
	freeHostFraction float64, usesContainers bool, containerPool *evergreen.ContainerPool, maxHosts int) (int, error) {

	evalStartAt := time.Now()
	if !d.IsEphemeral() {
		return 0, nil
	}
	if ctx.Err() != nil {
		return 0, errors.New("context canceled, not evaluating host utilization")
	}

	maxDuration := MaxDurationPerDistroHost
	///////////////////////////////////////////////////////////////////////
	// MaxDurationPerDistroHost = 30 * time.Minute
	///////////////////////////////////////////////////////////////////////
	if usesContainers {
		parentDistro, err := distro.FindOne(distro.ById(containerPool.Distro))
		if err != nil {
			return 0, errors.Wrap(err, "error finding parent distro")
		}
		maxDuration = MaxDurationPerDistroHostWithContainers
		///////////////////////////////////////////////////////////////////////
		// MaxDurationPerDistroHostWithContainers = 2 * time.Minute
		///////////////////////////////////////////////////////////////////////
		maxHosts = parentDistro.PoolSize * containerPool.MaxContainers
	}

	numNewHosts := 0

	// allocate 1 host per task that is longer than the max duration
	newTaskQueue, hostsForLongTasks := calcHostsForLongTasks(taskQueue, maxDuration)

	// determine the total expected running time of scheduled tasks
	scheduledTasksDuration := calcScheduledTasksDuration(newTaskQueue)

	// determine how many free hosts we have that are already up
	startAt := time.Now()
	numFreeHosts, err := calcExistingFreeHosts(existingHosts, freeHostFraction, maxDuration)
	if err != nil {
		return numNewHosts, err
	}
	freeHostDur := time.Since(startAt)

	// calculate how many new hosts are needed (minus the hosts for long tasks)
	numNewHosts = calcNewHostsNeeded(scheduledTasksDuration, maxDuration, numFreeHosts, hostsForLongTasks)

	// don't start more hosts than new tasks. This can happen if the task queue is mostly long tasks
	if numNewHosts > len(taskQueue) {
		numNewHosts = len(taskQueue)
	}

	// enforce the max hosts cap
	if isMaxHostsCapacity(maxHosts, containerPool, numNewHosts, len(existingHosts)) {
		numNewHosts = maxHosts - len(existingHosts)
	}

	// don't return negatives - can get here if over max hosts
	if numNewHosts < 0 {
		numNewHosts = 0
	}

	// alert if the distro is underwater
	if maxHosts < 1 {
		return 0, errors.Errorf("unable to plan hosts for distro %s due to pool size of %d", d.Id, d.PoolSize)
	}
	underWaterAlert := message.Fields{
		"provider":  d.Provider,
		"distro":    d.Id,
		"runtime":   scheduledTasksDuration,
		"runner":    RunnerName,
		"message":   "distro underwater",
		"num_hosts": len(existingHosts),
		"max_hosts": d.PoolSize,
	}
	avgMakespan := scheduledTasksDuration / time.Duration(maxHosts)
	grip.AlertWhen(avgMakespan > dynamicDistroRuntimeAlertThreshold, underWaterAlert)

	// log scheduler stats
	queueTasks := []string{}
	for _, t := range taskQueue {
		queueTasks = append(queueTasks, t.Id)
	}

	grip.Info(message.Fields{
		"message":                      "queue state report",
		"runner":                       RunnerName,
		"provider":                     d.Provider,
		"distro":                       d.Id,
		"pool_size":                    d.PoolSize,
		"new_hosts_needed":             numNewHosts,
		"num_existing_hosts":           len(existingHosts),
		"num_free_hosts_approx":        numFreeHosts,
		"queue_length":                 len(taskQueue),
		"queue_tasks":                  queueTasks,
		"long_tasks":                   hostsForLongTasks,
		"scheduled_tasks_runtime":      int64(scheduledTasksDuration),
		"scheduled_tasks_runtime_span": scheduledTasksDuration.String(),
		"op_dur_free_host_secs":        freeHostDur.Seconds(),
		"op_dur_total_secs":            time.Since(evalStartAt).Seconds(),
	})

	return numNewHosts, nil
}

func UtilizationBasedHostAllocator(ctx context.Context, hostAllocatorData HostAllocatorData) (int, error) {
	if hostAllocatorData == nil || hostAllocatorData.Distro == nil {

	}

	distro := hostAllocatorData.Distro
	if len(hostAllocatorData.existingHosts) >= distro.PoolSize {
		return 0, nil
	}

	// split tasks/hosts by task group (including those with no group) and find # of hosts needed for each
	newHostsNeeded := 0
	startAt := time.Now()
	taskGroupsData := groupByTaskGroup(hostAllocatorData.ExistingHosts, hostAllocatorData.DistroQueueInfo)
	taskGroupsDataRuntime := time.Since(startAt)
	startAt = time.Now()

	for taskGroupID, data := range taskGroupsData {
		var maxHosts int
		if taskGroupID == "" {
			maxHosts = distro.PoolSize
		} else {
			if len(data.info.TaskCount) == 0 {
				continue // skip this group if there are no tasks in the queue for it
			}
			maxHosts = data.info.MaxHosts
			///////////////////////////////////////////////////////////////////////
			// Who sets task.TaskGroupMaxHosts int `bson:"task_group_max_hosts,omitempty" json:"task_group_max_hosts,omitempty"`
			///////////////////////////////////////////////////////////////////////
		}

		// calculate number of hosts needed for this group
		newHosts, err := evalHostUtilization(ctx,
			distro,
			data.tasks,
			data.hosts,
			hostAllocatorData.freeHostFraction,
			hostAllocatorData.usesContainers,
			hostAllocatorData.containerPool,
			maxHosts)

		if err != nil {
			return 0, errors.Wrapf(err, "error calculating hosts for distro %s", distro.Id)
		}

		// add up total number of hosts needed for all groups
		newHostsNeeded += newHosts
	}

	calcRuntime := time.Since(startAt)

	grip.Info(message.Fields{
		"runner":         RunnerName,
		"distro":         distro.Id,
		"num_new_hosts":  newHostsNeeded,
		"message":        "requesting new hosts",
		"group_dur_secs": taskGroupsDataRuntime.Seconds(),
		"group_num":      len(taskGroupsData),
		"calc_dur_secs":  calcRuntime.Seconds(),
	})

	return newHostsNeeded, nil
}

// groupByTaskGroup takes a list of hosts and tasks and returns them grouped by task group
func groupByTaskGroup(runningHosts []host.Host, distroQueueInfo DistroQueueInfo) map[string]taskGroupData {
	taskGroupsData := map[string]taskGroupData{}
	for _, host := range runningHosts {
		task := host.RunningTask
		taskGroup := host.RunningTaskGroup
		buildVariant := host.RunningTaskBuildVariant
		project := host.RunningTaskProject
		version := host.RunningTaskVersion

		taskGroupID := ""
		if task != "" && taskGroup != "" {
			taskGroupID = fmt.Sprintf("%s_%s_%s_%s", taskGroup, buildVariant, project, version)
		}
		if data, exists := taskGroupsData[taskGroupID]; exists {
			data.hosts = append(data.hosts, host)
			taskGroupsData[taskGroupID].hosts = data.hosts
		} else {
			taskGroupsData[taskGroupID] = taskGroupData{
				hosts: []host.Host{host},
				info:  TaskGroupInfo{},
			}
		}
	}

	taskGroupsInfo := distroQueueInfo.TaskGroupsInfo
	for taskGroupID, info := range taskGroupsInfo {
		if _, exists := taskGroupsData[taskGroupID]; exists {
			taskGroupsData[taskGroupID].info = info
		} else {
			taskGroupsData[taskGroupID] = taskGroupData{
				info: info,
			}
		}
	}

	// Any task group can use a host not running a task group, so add them to each list.
	// This does mean that we can plan more than 1 task for a given host from 2 different
	// task groups, but that should be in the realm of "this is an estimate"
	for taskGroupID, data := range taskGroupsData {
		///////////////////////////////////////////////////////////////////////
		// Do we need to check if taskGroupsData[""] exists?
		///////////////////////////////////////////////////////////////////////
		if taskGroupID != "" {
			data.hosts = append(data.hosts, taskGroupsData[""].hosts...)
			taskGroupsData[taskGroupID] = data
		}
	}

	return taskGroupsData
}

// func UtilizationBasedHostAllocator(ctx context.Context, hostAllocatorData HostAllocatorData) (int, error) {
// 	if len(hostAllocatorData.existingHosts) >= hostAllocatorData.distro.PoolSize {
// 		return 0, nil
// 	}
//
// 	// split tasks/hosts by task group (including those with no group) and find # of hosts needed for each
// 	newHostsNeeded := 0
// 	startAt := time.Now()
// 	groupedData := groupByTaskGroup(hostAllocatorData.existingHosts, hostAllocatorData.taskQueueItems)
// 	///////////////////////////////////////////////////////////////////////
// 	// type taskGroupData struct {
// 	// 	hosts []host.Host
// 	// 	tasks []model.TaskQueueItem
// 	// }
// 	///////////////////////////////////////////////////////////////////////
// 	groupDataRuntime := time.Since(startAt)
// 	startAt = time.Now()
// 	for tg, data := range groupedData {
// 		var maxHosts int
// 		if tg == "" {
// 			maxHosts = hostAllocatorData.distro.PoolSize
// 		} else {
// 			if len(data.tasks) == 0 {
// 				continue // skip this group if there are no tasks in the queue for it
// 			}
// 			maxHosts = data.tasks[0].GroupMaxHosts
// 		}
//
// 		// calculate number of hosts needed for this group
// 		newHosts, err := evalHostUtilization(ctx,
// 			hostAllocatorData.distro,
// 			data.tasks,
// 			data.hosts,
// 			hostAllocatorData.freeHostFraction,
// 			hostAllocatorData.usesContainers,
// 			hostAllocatorData.containerPool,
// 			maxHosts)
//
// 		if err != nil {
// 			return 0, errors.Wrapf(err, "error calculating hosts for distro %s", hostAllocatorData.distro.Id)
// 		}
//
// 		// add up total number of hosts needed for all groups
// 		newHostsNeeded += newHosts
// 	}
// 	calcRuntime := time.Since(startAt)
//
// 	grip.Info(message.Fields{
// 		"runner":         RunnerName,
// 		"distro":         hostAllocatorData.distro.Id,
// 		"num_new_hosts":  newHostsNeeded,
// 		"message":        "requesting new hosts",
// 		"group_dur_secs": groupDataRuntime.Seconds(),
// 		"group_num":      len(groupedData),
// 		"calc_dur_secs":  calcRuntime.Seconds(),
// 	})
//
// 	return newHostsNeeded, nil
// }
//
// // groupByTaskGroup takes a list of hosts and tasks and returns them grouped by task group
// func groupByTaskGroup(runningHosts []host.Host, taskQueue []model.TaskQueueItem) map[string]taskGroupData {
// 	tgs := map[string]taskGroupData{}
// 	///////////////////////////////////////////////////////////////////////
// 	// type taskGroupData struct {
// 	// 	hosts []host.Host
// 	// 	tasks []model.TaskQueueItem
// 	// }
// 	///////////////////////////////////////////////////////////////////////
// 	for _, h := range runningHosts {
// 		groupString := ""
// 		if h.RunningTask != "" && h.RunningTaskGroup != "" {
// 			groupString = makeTaskGroupString(h.RunningTaskGroup, h.RunningTaskBuildVariant, h.RunningTaskProject, h.RunningTaskVersion)
// 			///////////////////////////////////////////////////////////////////////
// 			// return fmt.Sprintf("%s_%s_%s_%s", group, bv, project, version)
// 			///////////////////////////////////////////////////////////////////////
// 			// > db.hosts.find({"_id" : "ubuntu1804-z-8.maristisv.build.10gen.cc"},{"running_task": 1, "running_task_bv": 1, "running_task_project": 1, "running_task_version": 1}).limit(1).pretty()
// 			// {
// 			// 	"_id" : "ubuntu1804-z-8.maristisv.build.10gen.cc",
// 			// 	"running_task" : "mongodb_mongo_v4.0_enterprise_ubuntu1804_s390x_jsCore_patch_caa42a1f75a56c7643d0b68d3880444375ec42e3_5c59ae5ba4cf477981dca632_19_02_05_15_40_16",
// 			// 	"running_task_bv" : "enterprise-ubuntu1804-s390x",
// 			// 	"running_task_project" : "mongodb-mongo-v4.0",
// 			// 	"running_task_version" : "5c59ae5ba4cf477981dca632"
// 			// }
// 			///////////////////////////////////////////////////////////////////////
// 		}
//
// 		if data, exists := tgs[groupString]; exists {
// 			data.hosts = append(data.hosts, h)
// 			tgs[groupString] = data
// 		} else {
// 			tgs[groupString] = taskGroupData{
// 				hosts: []host.Host{h},
// 				tasks: []model.TaskQueueItem{},
// 			}
// 		}
// 	}
// 	for _, t := range taskQueue {
// 		groupString := ""
// 		if t.Group != "" {
// 			groupString = makeTaskGroupString(t.Group, t.BuildVariant, t.Project, t.Version)
//
// 		}
// 		if data, exists := tgs[groupString]; exists {
// 			data.tasks = append(data.tasks, t)
// 			tgs[groupString] = data
// 		} else {
// 			tgs[groupString] = taskGroupData{
// 				hosts: []host.Host{},
// 				tasks: []model.TaskQueueItem{t},
// 			}
// 		}
// 	}
//
// 	// Any task group can use a host not running a task group, so add them to each list.
// 	// This does mean that we can plan more than 1 task for a given host from 2 different
// 	// task groups, but that should be in the realm of "this is an estimate"
// 	for tg, data := range tgs {
// 		///////////////////////////////////////////////////////////////////////
// 		// tgs := map[string]taskGroupData{}
// 		///////////////////////////////////////////////////////////////////////
// 		if tg == "" {
// 			continue
// 		}
// 		data.hosts = append(data.hosts, tgs[""].hosts...)
// 		tgs[tg] = data
// 	}
//
// 	return tgs
// }

// func makeTaskGroupString(group, bv, project, version string) string {
// 	return fmt.Sprintf("%s_%s_%s_%s", group, bv, project, version)
// }

// Calculate the number of hosts needed by taking the total task scheduled task time
// and dividing it by the target duration. Request however many hosts are needed to
// achieve that minus the number of free hosts
// func evalHostUtilization(ctx context.Context, d distro.Distro, taskQueue []model.TaskQueueItem, existingHosts []host.Host,
// 	freeHostFraction float64, usesContainers bool, containerPool *evergreen.ContainerPool, maxHosts int) (int, error) {
//
// 	evalStartAt := time.Now()
// 	if !d.IsEphemeral() {
// 		return 0, nil
// 	}
// 	if ctx.Err() != nil {
// 		return 0, errors.New("context canceled, not evaluating host utilization")
// 	}
//
// 	maxDuration := MaxDurationPerDistroHost
// 	if usesContainers {
// 		parentDistro, err := distro.FindOne(distro.ById(containerPool.Distro))
// 		if err != nil {
// 			return 0, errors.Wrap(err, "error finding parent distro")
// 		}
// 		maxDuration = MaxDurationPerDistroHostWithContainers
// 		maxHosts = parentDistro.PoolSize * containerPool.MaxContainers
// 	}
//
// 	numNewHosts := 0
//
// 	// allocate 1 host per task that is longer than the max duration
// 	newTaskQueue, hostsForLongTasks := calcHostsForLongTasks(taskQueue, maxDuration)
//
// 	// determine the total expected running time of scheduled tasks
// 	scheduledTasksDuration := calcScheduledTasksDuration(newTaskQueue)
//
// 	// determine how many free hosts we have that are already up
// 	startAt := time.Now()
// 	numFreeHosts, err := calcExistingFreeHosts(existingHosts, freeHostFraction, maxDuration)
// 	if err != nil {
// 		return numNewHosts, err
// 	}
// 	freeHostDur := time.Since(startAt)
//
// 	// calculate how many new hosts are needed (minus the hosts for long tasks)
// 	numNewHosts = calcNewHostsNeeded(scheduledTasksDuration, maxDuration, numFreeHosts, hostsForLongTasks)
//
// 	// don't start more hosts than new tasks. This can happen if the task queue is mostly long tasks
// 	if numNewHosts > len(taskQueue) {
// 		numNewHosts = len(taskQueue)
// 	}
//
// 	// enforce the max hosts cap
// 	if isMaxHostsCapacity(maxHosts, containerPool, numNewHosts, len(existingHosts)) {
// 		numNewHosts = maxHosts - len(existingHosts)
// 	}
//
// 	// don't return negatives - can get here if over max hosts
// 	if numNewHosts < 0 {
// 		numNewHosts = 0
// 	}
//
// 	// alert if the distro is underwater
// 	if maxHosts < 1 {
// 		return 0, errors.Errorf("unable to plan hosts for distro %s due to pool size of %d", d.Id, d.PoolSize)
// 	}
// 	underWaterAlert := message.Fields{
// 		"provider":  d.Provider,
// 		"distro":    d.Id,
// 		"runtime":   scheduledTasksDuration,
// 		"runner":    RunnerName,
// 		"message":   "distro underwater",
// 		"num_hosts": len(existingHosts),
// 		"max_hosts": d.PoolSize,
// 	}
// 	avgMakespan := scheduledTasksDuration / time.Duration(maxHosts)
// 	grip.AlertWhen(avgMakespan > dynamicDistroRuntimeAlertThreshold, underWaterAlert)
//
// 	// log scheduler stats
// 	queueTasks := []string{}
// 	for _, t := range taskQueue {
// 		queueTasks = append(queueTasks, t.Id)
// 	}
//
// 	grip.Info(message.Fields{
// 		"message":                      "queue state report",
// 		"runner":                       RunnerName,
// 		"provider":                     d.Provider,
// 		"distro":                       d.Id,
// 		"pool_size":                    d.PoolSize,
// 		"new_hosts_needed":             numNewHosts,
// 		"num_existing_hosts":           len(existingHosts),
// 		"num_free_hosts_approx":        numFreeHosts,
// 		"queue_length":                 len(taskQueue),
// 		"queue_tasks":                  queueTasks,
// 		"long_tasks":                   hostsForLongTasks,
// 		"scheduled_tasks_runtime":      int64(scheduledTasksDuration),
// 		"scheduled_tasks_runtime_span": scheduledTasksDuration.String(),
// 		"op_dur_free_host_secs":        freeHostDur.Seconds(),
// 		"op_dur_total_secs":            time.Since(evalStartAt).Seconds(),
// 	})
//
// 	return numNewHosts, nil
// }

// calcScheduledTasksDuration returns the total estimated duration of all
// tasks scheduled to be run in a given task queue
func calcScheduledTasksDuration(queue []model.TaskQueueItem) time.Duration {
	var scheduledTasksDuration time.Duration

	for _, taskQueueItem := range queue {
		scheduledTasksDuration += taskQueueItem.ExpectedDuration
	}
	return scheduledTasksDuration
}

// calcNewHostsNeeded returns the number of new hosts needed based
// on a heuristic that utilizes the total duration scheduled tasks,
// targeting a maximum average duration for each task on all hosts
func calcNewHostsNeeded(scheduledDuration, maxDurationPerHost time.Duration, numExistingHosts, numHostsNeededAlready int) int {
	// number of hosts needed to meet the duration based turnaround requirement
	// may be a decimal because we care about the difference between 0.5 and 0 hosts
	numHostsForTurnaroundRequirement := float64(scheduledDuration) / float64(maxDurationPerHost)

	// number of hosts that need to be spun up
	numNewHostsNeeded := numHostsForTurnaroundRequirement - float64(numExistingHosts) + float64(numHostsNeededAlready)

	// if we need less than 1 new host but have no existing hosts, return 1 host
	// so that small queues are not stranded
	if numExistingHosts < 1 && numNewHostsNeeded > 0 && numNewHostsNeeded < 1 {
		return 1
	}

	// round the # of hosts needed down
	numNewHosts := int(math.Floor(numNewHostsNeeded))

	// return 0 if numNewHosts is less than 0
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return numNewHosts
}

// calcExistingFreeHosts returns the number of hosts that are not running a task,
// plus hosts that will soon be free scaled by some fraction
func calcExistingFreeHosts(existingHosts []host.Host, freeHostFactor float64, maxDurationPerHost time.Duration) (int, error) {
	numFreeHosts := 0
	if freeHostFactor > 1 {
		return numFreeHosts, errors.New("free host factor cannot be greater than 1")
	}

	for _, existingHost := range existingHosts {
		if existingHost.RunningTask == "" {
			numFreeHosts++
		}
	}

	soonToBeFree, err := getSoonToBeFreeHosts(existingHosts, freeHostFactor, maxDurationPerHost)
	if err != nil {
		return 0, err
	}

	return numFreeHosts + int(math.Floor(soonToBeFree)), nil
}

// getSoonToBeFreeHosts calculates a fractional number of hosts that are expected
// to be free for some fraction of the next maxDurationPerHost interval
// the final value is scaled by some fraction representing how confident we are that
// the hosts will actually be free in the expected amount of time
func getSoonToBeFreeHosts(existingHosts []host.Host, freeHostFactor float64, maxDurationPerHost time.Duration) (float64, error) {
	runningTaskIds := []string{}

	for _, existingDistroHost := range existingHosts {
		if existingDistroHost.RunningTask != "" {
			runningTaskIds = append(runningTaskIds, existingDistroHost.RunningTask)
		}
	}

	if len(runningTaskIds) == 0 {
		return 0.0, nil
	}

	runningTasks, err := task.Find(task.ByIds(runningTaskIds))
	if err != nil {
		return 0.0, err
	}

	nums := make(chan float64, len(runningTasks))
	source := make(chan task.Task, len(runningTasks))
	for _, t := range runningTasks {
		source <- t
	}
	close(source)

	wg := &sync.WaitGroup{}
	wg.Add(runtime.NumCPU())

	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic during free host calculation")
			defer wg.Done()
			for t := range source {
				expectedDuration := t.FetchExpectedDuration()
				elapsedTime := time.Since(t.StartTime)
				timeLeft := expectedDuration - elapsedTime

				// calculate what fraction of the host will be free within the max duration.
				// for example if we estimate 20 minutes left on the task and the target duration
				// for tasks is 30 minutes, assume that this host can be 1/3 of a free host
				freeHostFraction := float64(maxDurationPerHost-timeLeft) / float64(maxDurationPerHost)
				if freeHostFraction < 0 {
					freeHostFraction = 0
				}
				if freeHostFraction > 1 {
					freeHostFraction = 1
				}
				nums <- freeHostFactor * freeHostFraction // tune this by the free host factor
			}
		}()
	}
	wg.Wait()
	close(nums)

	var freeHosts float64
	for n := range nums {
		freeHosts += n
	}

	return freeHosts, nil
}

// calcHostsForLongTasks smooths out the task queue calculation by removing tasks
// longer than the max duration per host. This is so that a task that takes
// 3x as long as the max duration doesn't get 3 hosts allocated for it
func calcHostsForLongTasks(queue []model.TaskQueueItem, maxDurationPerHost time.Duration) ([]model.TaskQueueItem, int) {
	newQueue := []model.TaskQueueItem{}
	numRemoved := 0
	for _, queueItem := range queue {
		if queueItem.ExpectedDuration >= maxDurationPerHost {
			numRemoved++
		} else {
			newQueue = append(newQueue, queueItem)
		}
	}

	return newQueue, numRemoved
}

// isMaxHostsCapacity returns true if the max number of containers are already running
func isMaxHostsCapacity(maxHosts int, pool *evergreen.ContainerPool, numNewHosts, numExistingHosts int) bool {

	if pool != nil {
		if numNewHosts > (maxHosts*pool.MaxContainers)-numExistingHosts {
			return true
		}
	}

	if numNewHosts+numExistingHosts > maxHosts {
		return true
	}
	return false
}
