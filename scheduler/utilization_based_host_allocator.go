package scheduler

import (
	"context"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type TaskGroupData struct {
	Hosts []host.Host
	Info  model.TaskGroupInfo
}

func UtilizationBasedHostAllocator(ctx context.Context, hostAllocatorData *HostAllocatorData) (int, int, error) {
	distro := hostAllocatorData.Distro
	numExistingHosts := len(hostAllocatorData.ExistingHosts)
	minimumHostsThreshold := distro.HostAllocatorSettings.MinimumHosts

	// calculate approximate number of free hosts for the distro-scheduler-report
	freeHosts := make([]host.Host, 0, len(hostAllocatorData.ExistingHosts))
	for _, existingDistroHost := range hostAllocatorData.ExistingHosts {
		if existingDistroHost.RunningTask == "" {
			freeHosts = append(freeHosts, existingDistroHost)
		}
	}

	if distro.Provider != evergreen.ProviderNameDocker && numExistingHosts >= distro.HostAllocatorSettings.MaximumHosts {
		grip.Info(message.Fields{
			"message":        "distro is at max hosts",
			"distro":         distro.Id,
			"existing_hosts": numExistingHosts,
			"max_hosts":      distro.HostAllocatorSettings.MaximumHosts,
			"provider":       distro.Provider,
		})
		return 0, len(freeHosts), nil
	}

	// only want to meet minimum hosts
	if distro.Disabled {

		numNewHostsToRequest := minimumHostsThreshold - numExistingHosts

		if numNewHostsToRequest > 0 {
			grip.Info(message.Fields{
				"runner":                     RunnerName,
				"message":                    "requesting new hosts for disabled distro",
				"distro":                     distro.Id,
				"minimum_hosts_for_distro":   minimumHostsThreshold,
				"num_existing_hosts":         numExistingHosts,
				"total_new_hosts_to_request": numNewHostsToRequest,
			})
			return numNewHostsToRequest, len(freeHosts), nil
		}
		return 0, len(freeHosts), nil
	}

	// split tasks/hosts by task group (including those with no group) and find # of hosts needed for each
	taskGroupingBeginsAt := time.Now()
	taskGroupDatas := groupByTaskGroup(hostAllocatorData.ExistingHosts, hostAllocatorData.DistroQueueInfo)
	taskGroupingDuration := time.Since(taskGroupingBeginsAt)

	numNewHostsRequired := 0
	numFreeApprox := 0
	infoSliceIdx := make(map[string]int, len(hostAllocatorData.DistroQueueInfo.TaskGroupInfos))
	// get map of name:index
	for idx, info := range hostAllocatorData.DistroQueueInfo.TaskGroupInfos {
		infoSliceIdx[info.Name] = idx
	}
	for name, taskGroupData := range taskGroupDatas {
		var maxHosts int
		if name == "" {
			maxHosts = distro.HostAllocatorSettings.MaximumHosts
		} else {
			if taskGroupData.Info.Count == 0 {
				continue // skip this group if there are no tasks in the queue for it
			}
			maxHosts = taskGroupData.Info.MaxHosts
		}

		// calculate number of hosts needed for this group
		n, free, err := evalHostUtilization(
			ctx,
			distro,
			taskGroupData,
			distro.HostAllocatorSettings.FutureHostFraction,
			hostAllocatorData.ContainerPool,
			hostAllocatorData.DistroQueueInfo.MaxDurationThreshold,
			maxHosts)

		if err != nil {
			return 0, len(freeHosts), errors.Wrapf(err, "error calculating hosts for distro %s", distro.Id)
		}

		// add up total number of hosts needed for all groups
		numNewHostsRequired += n
		numFreeApprox += free
		if name != "" {
			hostAllocatorData.DistroQueueInfo.TaskGroupInfos[infoSliceIdx[name]].CountFree = free
			hostAllocatorData.DistroQueueInfo.TaskGroupInfos[infoSliceIdx[name]].CountRequired = n
		}
	}

	// Will at least distro.HostAllocatorSettings.MinimumHosts be running once numNewHostsRequired are up and running?
	numExistingAndRequiredHosts := numExistingHosts + numNewHostsRequired
	numAdditionalHostsToMeetMinimum := 0
	if numExistingAndRequiredHosts < minimumHostsThreshold {
		numAdditionalHostsToMeetMinimum = minimumHostsThreshold - numExistingAndRequiredHosts
	}
	numNewHostsToRequest := numNewHostsRequired + numAdditionalHostsToMeetMinimum

	return numNewHostsToRequest, numFreeApprox, nil
}

// Calculate the number of hosts needed by taking the total task scheduled task time
// and dividing it by the target duration. Request however many hosts are needed to
// achieve that minus the number of free hosts
func evalHostUtilization(ctx context.Context, d distro.Distro, taskGroupData TaskGroupData, futureHostFraction float64, containerPool *evergreen.ContainerPool, maxDurationThreshold time.Duration, maxHosts int) (int, int, error) {
	evalStartAt := time.Now()
	existingHosts := taskGroupData.Hosts
	taskGroupInfo := taskGroupData.Info
	numLongTasks := taskGroupInfo.CountDurationOverThreshold
	numOverdueTasks := taskGroupInfo.CountWaitOverThreshold
	scheduledDuration := taskGroupInfo.ExpectedDuration - taskGroupInfo.DurationOverThreshold
	numNewHosts := 0

	if !d.IsEphemeral() {
		return 0, 0, nil
	}
	// Why do we do this here?
	if ctx.Err() != nil {
		return 0, 0, errors.New("context canceled, not evaluating host utilization")
	}

	if containerPool != nil {
		parentDistro, err := distro.FindOneId(ctx, containerPool.Distro)
		if err != nil {
			return 0, 0, errors.Wrap(err, "error finding parent distros")
		}
		if parentDistro == nil {
			return 0, 0, errors.Errorf("distro '%s' not found", containerPool.Distro)
		}
		maxHosts = parentDistro.HostAllocatorSettings.MaximumHosts * containerPool.MaxContainers
	}

	// determine how many free hosts we have that are already up
	startAt := time.Now()
	numFreeHosts, err := calcExistingFreeHosts(existingHosts, futureHostFraction, maxDurationThreshold)
	if err != nil {
		return numNewHosts, numFreeHosts, err
	}
	freeHostDur := time.Since(startAt)

	roundDown := true
	if d.HostAllocatorSettings.RoundingRule == evergreen.HostAllocatorRoundUp {
		roundDown = false
	}
	numQOSTasks := numLongTasks

	if d.HostAllocatorSettings.FeedbackRule == evergreen.HostAllocatorWaitsOverThreshFeedback {
		numQOSTasks += numOverdueTasks
	}
	// calculate how many new hosts are needed (minus the hosts for long tasks)
	numNewHosts = calcNewHostsNeeded(scheduledDuration, maxDurationThreshold, numFreeHosts, numQOSTasks, roundDown)

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
		return 0, 0, errors.Errorf("unable to plan hosts for distro %s due to pool size of %d", d.Id, d.HostAllocatorSettings.MaximumHosts)
	}
	underWaterAlert := message.Fields{
		"provider":  d.Provider,
		"distro":    d.Id,
		"runtime":   scheduledDuration,
		"runner":    RunnerName,
		"message":   "distro underwater",
		"num_hosts": len(existingHosts),
		"max_hosts": d.HostAllocatorSettings.MaximumHosts,
	}
	avgMakespan := scheduledDuration / time.Duration(maxHosts)
	grip.AlertWhen(avgMakespan > dynamicDistroRuntimeAlertThreshold, underWaterAlert)

	return numNewHosts, numFreeHosts, nil
}

// groupByTaskGroup takes a list of hosts and tasks and returns them grouped by task group
func groupByTaskGroup(runningHosts []host.Host, distroQueueInfo model.DistroQueueInfo) map[string]TaskGroupData {
	taskGroupDatas := map[string]TaskGroupData{}
	for _, h := range runningHosts {
		name := ""
		if h.RunningTask != "" && h.RunningTaskGroup != "" {
			name = h.GetTaskGroupString()
		}
		if data, exists := taskGroupDatas[name]; exists {
			data.Hosts = append(data.Hosts, h)
			taskGroupDatas[name] = data
		} else {
			taskGroupDatas[name] = TaskGroupData{
				Hosts: []host.Host{h},
				Info:  model.TaskGroupInfo{},
			}
		}
	}

	taskGroupInfos := distroQueueInfo.TaskGroupInfos
	taskGroupInfosMap := make(map[string]model.TaskGroupInfo, len(taskGroupInfos))
	for _, info := range taskGroupInfos {
		taskGroupInfosMap[info.Name] = info
	}

	for name, info := range taskGroupInfosMap {
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

// calcNewHostsNeeded returns the number of new hosts needed based
// on a heuristic that utilizes the total duration of scheduled tasks.
// We should allocate enough hosts to run all tasks with runtime < maxDurationPerHost in less than maxDurationPerHost,
// and one host for each task with runtime > maxDurationPerHost.
// Alternatively, we should allocate sufficient hosts to schedule all tasks within maxDurationPerHost.
func calcNewHostsNeeded(scheduledDuration, maxDurationPerHost time.Duration, numExistingHosts, numHostsNeededAlready int, roundDown bool) int {
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

	// round the # of hosts needed down or up depending

	var numNewHosts int
	if roundDown {
		numNewHosts = int(math.Floor(numNewHostsNeeded))
	} else {
		numNewHosts = int(math.Ceil(numNewHostsNeeded))
	}

	// return 0 if numNewHosts is less than 0
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return numNewHosts
}

// calcExistingFreeHosts returns the number of hosts that are not running a task,
// plus hosts that will soon be free scaled by some fraction
func calcExistingFreeHosts(existingHosts []host.Host, futureHostFactor float64, maxDurationPerHost time.Duration) (int, error) {
	numFreeHosts := 0
	if futureHostFactor > 1 {
		return numFreeHosts, errors.New("future host factor cannot be greater than 1")
	}

	for _, existingHost := range existingHosts {
		if existingHost.RunningTask == "" {
			numFreeHosts++
		}
	}

	soonToBeFree, err := getSoonToBeFreeHosts(existingHosts, futureHostFactor, maxDurationPerHost)
	if err != nil {
		return 0, err
	}

	return numFreeHosts + int(math.Floor(soonToBeFree)), nil
}

// getSoonToBeFreeHosts calculates a fractional number of hosts that are expected
// to be free for some fraction of the next maxDurationPerHost interval
// the final value is scaled by some fraction representing how confident we are that
// the hosts will actually be free in the expected amount of time
func getSoonToBeFreeHosts(existingHosts []host.Host, futureHostFraction float64, maxDurationPerHost time.Duration) (float64, error) {
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
			defer recovery.LogStackTraceAndContinue("panic during future free host calculation")
			defer wg.Done()
			for t := range source {
				durationStats := t.FetchExpectedDuration()
				expectedDuration := durationStats.Average
				durationStdDev := durationStats.StdDev
				elapsedTime := time.Since(t.StartTime)
				timeLeft := expectedDuration - elapsedTime

				// calculate what fraction of the host will be free within the max duration.
				// for example if we estimate 20 minutes left on the task and the target duration
				// for tasks is 30 minutes, assume that this host can be 1/3 of a free host
				var fractionalHostFree float64
				if elapsedTime > evergreen.MaxDurationPerDistroHost && durationStdDev > 0 && elapsedTime > expectedDuration+3*durationStdDev {
					// if the task has taken over 3 std deviations longer (so longer than 99.7% of past runs),
					// assume that the host will be busy the entire duration. This is to avoid unusually long
					// tasks preventing us from starting any hosts
					fractionalHostFree = 0
				} else {
					fractionalHostFree = float64(maxDurationPerHost-timeLeft) / float64(maxDurationPerHost)
				}
				if fractionalHostFree < 0 {
					fractionalHostFree = 0
				}
				if fractionalHostFree > 1 {
					fractionalHostFree = 1
				}
				nums <- futureHostFraction * fractionalHostFree // tune this by the future host fraction
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
