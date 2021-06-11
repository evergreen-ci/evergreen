package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	hostAllocatorJobName = "host-allocator"
)

func init() {
	registry.AddJobType(hostAllocatorJobName, func() amboy.Job {
		return makeHostAllocatorJob()
	})
}

type hostAllocatorJob struct {
	DistroID string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	env evergreen.Environment
}

func makeHostAllocatorJob() *hostAllocatorJob {
	job := &hostAllocatorJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostAllocatorJobName,
				Version: 0,
			},
		},
	}

	job.SetDependency(dependency.NewAlways())

	return job
}

func NewHostAllocatorJob(env evergreen.Environment, distroID string, timestamp time.Time) amboy.Job {
	j := makeHostAllocatorJob()
	j.DistroID = distroID
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s.%s", hostAllocatorJobName, distroID, timestamp.Format(TSFormat)))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", hostAllocatorJobName, distroID)})
	j.SetShouldApplyScopesOnEnqueue(true)

	return j
}

func (j *hostAllocatorJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	config, err := evergreen.GetConfig()
	if err != nil {
		j.AddError(errors.Wrap(err, "Can't get evergreen configuration"))
		return
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrapf(err, "Can't get degraded mode flags"))
		return
	}

	if flags.HostAllocatorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     hostAllocatorJobName,
			"message": "host allocation is disabled",
		})
		return
	}

	distro, err := distro.FindByIdWithDefaultSettings(j.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "Database error for find() by distro id '%s'", j.DistroID))
		return
	}
	if distro == nil {
		j.AddError(errors.Errorf("distro '%s' not found", j.DistroID))
		return
	}
	if _, err = distro.GetResolvedHostAllocatorSettings(config); err != nil {
		j.AddError(errors.Errorf("distro '%s' host allocator settings failed to resolve", j.DistroID))
		return
	}

	if err = scheduler.UpdateStaticDistro(*distro); err != nil {
		j.AddError(errors.Wrap(err, "problem updating static hosts"))
		return
	}

	var containerPool *evergreen.ContainerPool
	if distro.ContainerPool != "" {
		containerPool = config.ContainerPools.GetContainerPool(distro.ContainerPool)
		if containerPool == nil {
			j.AddError(errors.Wrapf(err, "Distro container pool not found for distro id '%s'", j.DistroID))
			return
		}
	}

	if err = host.RemoveStaleInitializing(j.DistroID); err != nil {
		j.AddError(errors.Wrap(err, "Problem removing previous intent hosts before creating new ones"))
		return
	}

	existingHosts, err := host.AllActiveHosts(j.DistroID)
	if err != nil {
		j.AddError(errors.Wrap(err, "Database error retrieving running hosts"))
		return
	}
	upHosts := existingHosts.Uphosts()

	distroQueueInfo, err := model.GetDistroQueueInfo(j.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "Database error retrieving DistroQueueInfo for distro id '%s'", j.DistroID))
		return
	}

	////////////////////////
	// host-allocation phase
	////////////////////////

	hostAllocationBegins := time.Now()

	hostAllocator := scheduler.GetHostAllocator(config.Scheduler.HostAllocator)

	hostAllocatorData := scheduler.HostAllocatorData{
		Distro:          *distro,
		ExistingHosts:   upHosts,
		UsesContainers:  (containerPool != nil),
		ContainerPool:   containerPool,
		DistroQueueInfo: distroQueueInfo,
	}

	// nHosts is the number of additional hosts desired.
	nHosts, nHostsFree, err := hostAllocator(ctx, &hostAllocatorData)
	if err != nil {
		j.AddError(errors.Wrapf(err, "Error calculating the number of new hosts required for distro id '%s'", j.DistroID))
		return
	}

	grip.Info(message.Fields{
		"runner":        hostAllocatorJobName,
		"distro":        j.DistroID,
		"operation":     "runtime-stats",
		"phase":         "host-allocation",
		"instance":      j.ID(),
		"duration_secs": time.Since(hostAllocationBegins).Seconds(),
	})

	//////////////////////
	// host-spawning phase
	//////////////////////

	hostSpawningBegins := time.Now()
	hostsSpawned, err := scheduler.SpawnHosts(ctx, *distro, nHosts, containerPool)
	if err != nil {
		j.AddError(errors.Wrap(err, "Error spawning new hosts"))
		return
	}

	eventInfo := event.TaskQueueInfo{
		TaskQueueLength:  distroQueueInfo.Length,
		NumHostsRunning:  len(upHosts),
		ExpectedDuration: distroQueueInfo.ExpectedDuration,
	}

	event.LogSchedulerEvent(event.SchedulerEventData{
		TaskQueueInfo: eventInfo,
		DistroId:      distro.Id,
	})

	grip.Info(message.Fields{
		"runner":        hostAllocatorJobName,
		"distro":        distro.Id,
		"operation":     "runtime-stats",
		"phase":         "host-spawning",
		"instance":      j.ID(),
		"duration_secs": time.Since(hostSpawningBegins).Seconds(),
	})

	// ignoring all the tasks that will take longer than the threshold to run,
	// and the hosts allocated for them,
	// how long will it take the current fleet of hosts, plus the ones we spawned, to chew through
	// the scheduled tasks in the queue?

	var totalOverdueInTaskGroups, countDurationOverThresholdInTaskGroups, freeInTaskGroups, requiredInTaskGroups int
	var durationOverThresholdInTaskGroups, expectedDurationInTaskGroups time.Duration
	for _, info := range hostAllocatorData.DistroQueueInfo.TaskGroupInfos {
		if info.Name != "" {
			totalOverdueInTaskGroups += info.CountWaitOverThreshold
			countDurationOverThresholdInTaskGroups += info.CountDurationOverThreshold
			durationOverThresholdInTaskGroups += info.DurationOverThreshold
			expectedDurationInTaskGroups += info.ExpectedDuration
			freeInTaskGroups += info.CountFree
			requiredInTaskGroups += info.CountRequired
		}
	}

	correctedExpectedDuration := distroQueueInfo.ExpectedDuration - expectedDurationInTaskGroups
	correctedDurationOverThreshold := distroQueueInfo.DurationOverThreshold - durationOverThresholdInTaskGroups
	scheduledDuration := correctedExpectedDuration - correctedDurationOverThreshold
	durationOverThreshNoTaskGroups := distroQueueInfo.CountDurationOverThreshold - countDurationOverThresholdInTaskGroups

	correctedHostsSpawned := len(hostsSpawned) - requiredInTaskGroups
	hostsAvail := (nHostsFree - freeInTaskGroups) + correctedHostsSpawned - durationOverThreshNoTaskGroups

	var timeToEmpty, timeToEmptyNoSpawns time.Duration
	if scheduledDuration <= 0 {
		timeToEmpty = time.Duration(0)
		timeToEmptyNoSpawns = time.Duration(0)
	} else {
		// this is roughly the maximum value of hours we can set a time.Duration to
		const maxPossibleHours = 2532000
		hostsAvailNoSpawns := hostsAvail - correctedHostsSpawned
		if hostsAvail <= 0 {
			timeToEmpty = time.Duration(maxPossibleHours) * time.Hour
			timeToEmptyNoSpawns = time.Duration(maxPossibleHours) * time.Hour
		} else if hostsAvailNoSpawns <= 0 {
			timeToEmpty = scheduledDuration / time.Duration(hostsAvail)
			timeToEmptyNoSpawns = time.Duration(maxPossibleHours) * time.Hour
		} else {
			timeToEmpty = scheduledDuration / time.Duration(hostsAvail)
			timeToEmptyNoSpawns = scheduledDuration / time.Duration(hostsAvailNoSpawns)
		}
	}

	hostQueueRatio := float32(timeToEmpty.Nanoseconds()) / float32(distroQueueInfo.MaxDurationThreshold.Nanoseconds())
	noSpawnsRatio := float32(timeToEmptyNoSpawns.Nanoseconds()) / float32(distroQueueInfo.MaxDurationThreshold.Nanoseconds())

	// rough value that should correspond to situations where a queue will be empty very soon
	const lowRatioThresh = float32(.25)
	terminateExcess := distro.HostAllocatorSettings.HostsOverallocatedRule == evergreen.HostsOverallocatedTerminate
	if terminateExcess && hostQueueRatio < lowRatioThresh && len(upHosts) > 0 {
		distroIsByHour := cloud.UsesHourlyBilling(&upHosts[0].Distro)
		if !distroIsByHour {
			j.setTargetAndTerminate(ctx, len(upHosts), hostQueueRatio, distro)
		}
	}

	grip.Info(message.Fields{
		"message":                      "distro-scheduler-report",
		"job_type":                     hostAllocatorJobName,
		"distro":                       distro.Id,
		"provider":                     distro.Provider,
		"max_hosts":                    distro.HostAllocatorSettings.MaximumHosts,
		"num_new_hosts":                len(hostsSpawned),
		"pool_info":                    existingHosts.Stats(),
		"queue":                        eventInfo,
		"overdue_tasks":                distroQueueInfo.CountWaitOverThreshold,
		"overdue_tasks_in_groups":      totalOverdueInTaskGroups,
		"total_runtime":                distroQueueInfo.ExpectedDuration.String(),
		"runtime_secs":                 distroQueueInfo.ExpectedDuration.Seconds(),
		"time_to_empty":                timeToEmpty.String(),
		"time_to_empty_mins":           timeToEmpty.Minutes(),
		"time_to_empty_no_spawns":      timeToEmptyNoSpawns.String(),
		"time_to_empty_no_spawns_mins": timeToEmptyNoSpawns.Minutes(),
		"host_queue_ratio":             hostQueueRatio,
		"host_queue_ratio_no_spawns":   noSpawnsRatio,
		"instance":                     j.ID(),
		"runner":                       scheduler.RunnerName,
	})
}

func (j *hostAllocatorJob) setTargetAndTerminate(ctx context.Context, numUpHosts int, hostQueueRatio float32, distro *distro.Distro) {
	var killableHosts, newCapTarget int
	if hostQueueRatio == 0 {
		killableHosts = numUpHosts
	} else {
		killableHosts = int(float32(numUpHosts) * (1 - hostQueueRatio))
		newCapTarget = numUpHosts - killableHosts
	}

	if newCapTarget < distro.HostAllocatorSettings.MinimumHosts {
		newCapTarget = distro.HostAllocatorSettings.MinimumHosts
	}
	// rough value to prevent killing hosts on low-volume distros
	const lowCountFloor = 0
	if killableHosts > lowCountFloor {

		drawdownInfo := DrawdownInfo{
			DistroID:     distro.Id,
			NewCapTarget: newCapTarget,
		}
		err := amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), NewHostDrawdownJob(j.env, drawdownInfo, utility.RoundPartOfMinute(1).Format(TSFormat)))
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "Error drawing down hosts",
				"instance": j.ID(),
				"distro":   distro.Id,
			}))
		}

	}

}
