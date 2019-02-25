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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
)

type TaskGroupData struct {
	Hosts []host.Host
	Info  TaskGroupInfo
}

func UtilizationBasedHostAllocator(ctx context.Context, hostAllocatorData HostAllocatorData) (int, error) {
	distro := hostAllocatorData.Distro
	if len(hostAllocatorData.ExistingHosts) >= distro.PoolSize {
		return 0, nil
	}

	// split tasks/hosts by task group (including those with no group) and find # of hosts needed for each
	newHostsNeeded := 0
	startAt := time.Now()
	taskGroupDatas := groupByTaskGroup(hostAllocatorData.ExistingHosts, hostAllocatorData.DistroQueueInfo)
	taskGroupingRuntime := time.Since(startAt)
	startAt = time.Now()

	for name, taskGroupData := range taskGroupDatas {
		var maxHosts int
		if name == "" {
			maxHosts = distro.PoolSize
		} else {
			if taskGroupData.Info.Count == 0 {
				continue // skip this group if there are no tasks in the queue for it
			}
			maxHosts = taskGroupData.Info.MaxHosts
		}

		// calculate number of hosts needed for this group
		newHosts, err := evalHostUtilization(
			ctx,
			distro,
			taskGroupData,
			hostAllocatorData.FreeHostFraction,
			hostAllocatorData.ContainerPool,
			hostAllocatorData.MaxDurationThreshold,
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
		"group_dur_secs": taskGroupingRuntime.Seconds(),
		"group_num":      len(taskGroupDatas),
		"calc_dur_secs":  calcRuntime.Seconds(),
	})

	return newHostsNeeded, nil
}

// Calculate the number of hosts needed by taking the total task scheduled task time
// and dividing it by the target duration. Request however many hosts are needed to
// achieve that minus the number of free hosts
func evalHostUtilization(ctx context.Context, d distro.Distro, taskGroupData TaskGroupData, freeHostFraction float64, containerPool *evergreen.ContainerPool, maxDurationThreshold time.Duration, maxHosts int) (int, error) {
	evalStartAt := time.Now()
	existingHosts := taskGroupData.Hosts
	taskGroupInfo := taskGroupData.Info
	numLongTasks := taskGroupInfo.CountOverThreshold
	scheduledDuration := taskGroupInfo.ExpectedDuration - taskGroupInfo.DurationOverThreshold
	numNewHosts := 0

	if !d.IsEphemeral() {
		return 0, nil
	}
	// Why do we do this here?
	if ctx.Err() != nil {
		return 0, errors.New("context canceled, not evaluating host utilization")
	}

	if containerPool != nil {
		parentDistro, err := distro.FindOne(distro.ById(containerPool.Distro))
		if err != nil {
			return 0, errors.Wrap(err, "error finding parent distro")
		}
		maxHosts = parentDistro.PoolSize * containerPool.MaxContainers
	}

	// determine how many free hosts we have that are already up
	startAt := time.Now()
	numFreeHosts, err := calcExistingFreeHosts(existingHosts, freeHostFraction, maxDurationThreshold)
	if err != nil {
		return numNewHosts, err
	}
	freeHostDur := time.Since(startAt)

	// calculate how many new hosts are needed (minus the hosts for long tasks)
	numNewHosts = calcNewHostsNeeded(scheduledDuration, maxDurationThreshold, numFreeHosts, numLongTasks)

	// don't start more hosts than new tasks. This can happen if the task queue is mostly long tasks
	if numNewHosts > taskGroupInfo.Count {
		numNewHosts = taskGroupInfo.Count
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
		"runtime":   scheduledDuration,
		"runner":    RunnerName,
		"message":   "distro underwater",
		"num_hosts": len(existingHosts),
		"max_hosts": d.PoolSize,
	}
	avgMakespan := scheduledDuration / time.Duration(maxHosts)
	grip.AlertWhen(avgMakespan > dynamicDistroRuntimeAlertThreshold, underWaterAlert)

	grip.Info(message.Fields{
		"message":                      "queue state report",
		"runner":                       RunnerName,
		"provider":                     d.Provider,
		"distro":                       d.Id,
		"pool_size":                    d.PoolSize,
		"new_hosts_needed":             numNewHosts,
		"num_existing_hosts":           len(existingHosts),
		"num_free_hosts_approx":        numFreeHosts,
		"queue_length":                 taskGroupInfo.Count,
		"long_tasks":                   numLongTasks,
		"scheduled_tasks_runtime":      int64(scheduledDuration),
		"scheduled_tasks_runtime_span": scheduledDuration.String(),
		"op_dur_free_host_secs":        freeHostDur.Seconds(),
		"op_dur_total_secs":            time.Since(evalStartAt).Seconds(),
	})

	return numNewHosts, nil
}

// groupByTaskGroup takes a list of hosts and tasks and returns them grouped by task group
func groupByTaskGroup(runningHosts []host.Host, distroQueueInfo DistroQueueInfo) map[string]TaskGroupData {
	taskGroupDatas := map[string]TaskGroupData{}
	for _, h := range runningHosts {
		name := ""
		if h.RunningTask != "" && h.RunningTaskGroup != "" {
			name = makeTaskGroupString(h.RunningTaskGroup, h.RunningTaskBuildVariant, h.RunningTaskProject, h.RunningTaskVersion)
		}
		if data, exists := taskGroupDatas[name]; exists {
			data.Hosts = append(data.Hosts, h)
			taskGroupDatas[name] = data
		} else {
			taskGroupDatas[name] = TaskGroupData{
				Hosts: []host.Host{h},
				Info:  TaskGroupInfo{},
			}
		}
	}
	taskGroupsInfosMap := distroQueueInfo.taskGroupInfosMap
	for name, info := range taskGroupsInfosMap {
		if data, exists := taskGroupDatas[name]; exists {
			data.Info = info
			taskGroupDatas[name] = data
		} else {
			taskGroupDatas[name] = TaskGroupData{
				Hosts: []host.Host{},
				Info:  info,
			}
		}
	}

	// Any task group can use a host not running a task group, so add them to each list.
	// This does mean that we can plan more than 1 task for a given host from 2 different
	// task groups, but that should be in the realm of "this is an estimate"
	if taskGroupData, ok := taskGroupDatas[""]; ok {
		for name, data := range taskGroupDatas {
			if name == "" {
				continue
			}
			data.Hosts = append(data.Hosts, taskGroupData.Hosts...)
			taskGroupDatas[name] = data
		}
	}

	return taskGroupDatas
}

func makeTaskGroupString(group, bv, project, version string) string {
	return fmt.Sprintf("%s_%s_%s_%s", group, bv, project, version)
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
