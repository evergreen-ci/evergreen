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
		if existingDistroHost.IsFree() {
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

	// If the distro is disabled, we only need to meet the minimum number of hosts
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
	taskGroupDatas := groupByTaskGroup(hostAllocatorData.ExistingHosts, hostAllocatorData.DistroQueueInfo)

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

	// Ensure we do not request more hosts than there are dependency-fulfilled tasks
	if numNewHostsRequired+len(freeHosts) > hostAllocatorData.DistroQueueInfo.LengthWithDependenciesMet {
		numNewHostsRequired = hostAllocatorData.DistroQueueInfo.LengthWithDependenciesMet - len(freeHosts)
	}
	if numNewHostsRequired < 0 {
		numNewHostsRequired = 0
	}

	// Ensure that at least distro.HostAllocatorSettings.MinimumHosts will be running once numNewHostsRequired are up and running.
	numExistingAndRequiredHosts := numExistingHosts + numNewHostsRequired
	numAdditionalHostsToMeetMinimum := 0
	if numExistingAndRequiredHosts < minimumHostsThreshold {
		numAdditionalHostsToMeetMinimum = minimumHostsThreshold - numExistingAndRequiredHosts
	}
	numNewHostsToRequest := numNewHostsRequired + numAdditionalHostsToMeetMinimum

	return numNewHostsToRequest, numFreeApprox, nil
}

// evalHostUtilization calculates the number of hosts needed by taking the total task scheduled task time
// and dividing it by the target duration. Request however many hosts are needed to achieve that minus the
// number of free hosts
func evalHostUtilization(ctx context.Context, d distro.Distro, taskGroupData TaskGroupData, futureHostFraction float64, containerPool *evergreen.ContainerPool, maxDurationThreshold time.Duration, maxHosts int) (int, int, error) {
	existingHosts := taskGroupData.Hosts
	taskGroupInfo := taskGroupData.Info
	numLongRunningTasks := taskGroupInfo.CountDurationOverThreshold
	totalShortRunningTasksExpectedDuration := taskGroupInfo.ExpectedDuration - taskGroupInfo.DurationOverThreshold
	numNewHosts := 0

	// Skip for non-static providers
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

	// Determine the number of expected free hosts by summing the number of free hosts with the
	// estimated number of hosts we expect to be free within the maxDurationThreshold. This estimation
	// is calculated by taking all running tasks that are expected to complete within maxDurationThreshold,
	// summing their estimated time left to completion, and dividing that number by maxDurationThreshold.
	// That estimate is then multiplied by the futureHostFraction coefficient, which is a fraction that allows us
	// to tune the final estimate up or down.
	expectedNumFreeHosts, err := calcExistingFreeHosts(ctx, existingHosts, futureHostFraction, maxDurationThreshold)
	if err != nil {
		return numNewHosts, expectedNumFreeHosts, err
	}

	roundDown := true
	if d.HostAllocatorSettings.RoundingRule == evergreen.HostAllocatorRoundUp {
		roundDown = false
	}

	numHostsForOverdueTasks := 0
	if d.HostAllocatorSettings.FeedbackRule == evergreen.HostAllocatorWaitsOverThreshFeedback {
		numHostsForOverdueTasks = taskGroupInfo.CountWaitOverThreshold
	}
	numNewHosts = calcNewHostsNeeded(totalShortRunningTasksExpectedDuration, maxDurationThreshold, expectedNumFreeHosts, numLongRunningTasks, numHostsForOverdueTasks, roundDown)

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

	if maxHosts < 1 {
		return 0, 0, errors.Errorf("unable to plan hosts for distro %s due to pool size of %d", d.Id, d.HostAllocatorSettings.MaximumHosts)
	}
	// Alert if we expect the total expected duration of all short running tasks to take over
	// the dynamicDistroRuntimeAlertThreshold even when the distro's (or group's) max hosts
	// are reached
	underWaterAlert := message.Fields{
		"provider":        d.Provider,
		"distro":          d.Id,
		"runtime":         totalShortRunningTasksExpectedDuration,
		"runner":          RunnerName,
		"message":         "distro underwater",
		"num_hosts":       len(existingHosts),
		"max_hosts":       d.HostAllocatorSettings.MaximumHosts,
		"task_group_name": taskGroupInfo.Name,
	}
	avgMakespan := totalShortRunningTasksExpectedDuration / time.Duration(maxHosts)
	grip.AlertWhen(avgMakespan > dynamicDistroRuntimeAlertThreshold, underWaterAlert)

	return numNewHosts, expectedNumFreeHosts, nil
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

	return taskGroupDatas
}

// calcNewHostsNeeded returns the number of new hosts needed based on a heuristic that utilizes the
// sum of the expected durations of all short-running (<= maxDurationPerHost) tasks that have their
// dependencies met. It attempts to allocate enough hosts to run all short running tasks within
// maxDurationPerHost, plus one host for each long-running task with runtime > maxDurationPerHost,
// plus (optionally) one host for each task that have been waiting maxDurationPerHost since its dependencies
// were met.
func calcNewHostsNeeded(totalShortRunningTasksExpectedDuration, maxDurationPerHost time.Duration,
	expectedNumFreeHosts, numLongRunningTasks, numHostsForOverdueTasks int, roundDown bool) int {

	// Calculate the number of hosts needed to run the full totalShortRunningTasksExpectedDuration within
	// the maxDurationPerHost turnaround requirement
	numHostsForTurnaroundRequirement := float64(totalShortRunningTasksExpectedDuration) / float64(maxDurationPerHost)

	// Subtract the number of hosts that we expect to be free, add the number of long-running tasks,
	// and add the number of overdue tasks to get the final number of hosts that need to be spun up
	numNewHostsNeeded := numHostsForTurnaroundRequirement - float64(expectedNumFreeHosts) + float64(numLongRunningTasks) + float64(numHostsForOverdueTasks)

	// If we need less than 1 new host but have no existing hosts, return 1 host
	// so that small queues are not stranded
	if expectedNumFreeHosts < 1 && numNewHostsNeeded > 0 && numNewHostsNeeded < 1 {
		return 1
	}

	// Round the number of hosts needed down or up
	var numNewHosts int
	if roundDown {
		numNewHosts = int(math.Floor(numNewHostsNeeded))
	} else {
		numNewHosts = int(math.Ceil(numNewHostsNeeded))
	}
	if numNewHosts < 0 {
		numNewHosts = 0
	}
	return numNewHosts
}

// calcExistingFreeHosts returns the number of hosts that are not running a task,
// plus hosts that will soon be free scaled by some fraction
func calcExistingFreeHosts(ctx context.Context, existingHosts []host.Host, futureHostFactor float64, maxDurationPerHost time.Duration) (int, error) {
	numFreeHosts := 0
	if futureHostFactor > 1 {
		return numFreeHosts, errors.New("future host factor cannot be greater than 1")
	}

	for _, existingHost := range existingHosts {
		if existingHost.IsFree() {
			numFreeHosts++
		}
	}

	soonToBeFree, err := getSoonToBeFreeHosts(ctx, existingHosts, futureHostFactor, maxDurationPerHost)
	if err != nil {
		return 0, err
	}

	return numFreeHosts + int(math.Floor(soonToBeFree)), nil
}

// getSoonToBeFreeHosts calculates a fractional number of hosts that are expected
// to be free for some fraction of the next maxDurationPerHost interval
// the final value is scaled by some fraction representing how confident we are that
// the hosts will actually be free in the expected amount of time
func getSoonToBeFreeHosts(ctx context.Context, existingHosts []host.Host, futureHostFraction float64, maxDurationPerHost time.Duration) (float64, error) {
	runningTaskIds := []string{}

	for _, existingDistroHost := range existingHosts {
		if existingDistroHost.RunningTask != "" {
			runningTaskIds = append(runningTaskIds, existingDistroHost.RunningTask)
		}
	}

	if len(runningTaskIds) == 0 {
		return 0.0, nil
	}

	runningTasks, err := task.Find(ctx, task.ByIds(runningTaskIds))
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
	wg.Add(runtime.GOMAXPROCS(0))

	for i := 0; i < runtime.GOMAXPROCS(0); i++ {
		go func() {
			defer recovery.LogStackTraceAndContinue("panic during future free host calculation")
			defer wg.Done()
			for t := range source {
				durationStats := t.FetchExpectedDuration(ctx)
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
