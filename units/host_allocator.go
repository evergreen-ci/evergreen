package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/hoststat"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	hostAllocatorJobName         = "host-allocator"
	hostAllocatorAttributePrefix = "evergreen.host_allocator"
	maxHostAllocatorJobTime      = 10 * time.Minute
	// maxIntentHosts represents the maximum number of intent hosts we can
	// be processing at once, in order to prevent over-logging
	maxIntentHosts = 5000
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
	return job
}

func NewHostAllocatorJob(env evergreen.Environment, distroID string, timestamp time.Time) amboy.Job {
	j := makeHostAllocatorJob()
	j.DistroID = distroID
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s.%s", hostAllocatorJobName, distroID, timestamp.Format(TSFormat)))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", hostAllocatorJobName, distroID)})
	j.SetEnqueueAllScopes(true)
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxHostAllocatorJobTime,
	})

	return j
}

func (j *hostAllocatorJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting admin settings"))
		return
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting service flags"))
		return
	}

	if flags.HostAllocatorDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     hostAllocatorJobName,
			"message": "host allocation is disabled",
		})
		return
	}

	distro, err := distro.FindByIdWithDefaultSettings(ctx, j.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding distro '%s'", j.DistroID))
		return
	}
	if distro == nil {
		j.AddError(errors.Errorf("distro '%s' not found", j.DistroID))
		return
	}
	if _, err = distro.GetResolvedHostAllocatorSettings(config); err != nil {
		j.AddError(errors.Errorf("resolving distro '%s' host allocator settings", j.DistroID))
		return
	}

	if err = scheduler.UpdateStaticDistro(ctx, *distro); err != nil {
		j.AddError(errors.Wrapf(err, "updating static host in distro '%s'", j.DistroID))
		return
	}

	var containerPool *evergreen.ContainerPool
	if distro.ContainerPool != "" {
		containerPool = config.ContainerPools.GetContainerPool(distro.ContainerPool)
		if containerPool == nil {
			j.AddError(errors.Wrapf(err, "container pool not found for distro '%s'", j.DistroID))
			return
		}
	}

	if err = host.RemoveStaleInitializing(ctx, j.DistroID); err != nil {
		j.AddError(errors.Wrap(err, "removing stale initializing intent hosts"))
		return
	}
	if err = host.MarkStaleBuildingAsFailed(ctx, j.DistroID); err != nil {
		j.AddError(errors.Wrap(err, "marking building intent hosts as failed"))
		return
	}

	if distro.Disabled {
		return
	}

	////////////////////////
	// host-allocation phase
	////////////////////////

	distroQueueInfo, err := model.GetDistroQueueInfo(ctx, j.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting distro queue info for distro '%s'", j.DistroID))
		return
	}

	existingHosts, err := host.AllActiveHosts(ctx, j.DistroID)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding active hosts"))
		return
	}

	hostAllocationBegins := time.Now()

	// Total number of hosts with a status within evergreen.UpHostStatus
	upHosts := existingHosts.Uphosts()
	// Total number of hosts that started provisioning but are not yet up
	provisioningHosts := existingHosts.ProvisioningHosts()

	var nHosts, nHostsFree int
	hostAllocatorData := scheduler.HostAllocatorData{
		Distro:          *distro,
		ExistingHosts:   upHosts,
		UsesContainers:  (containerPool != nil),
		ContainerPool:   containerPool,
		DistroQueueInfo: distroQueueInfo,
	}

	j.saveHostStats(ctx, distro, len(upHosts))

	if distro.SingleTaskDistro {
		// Single tasks distros should spawn a host for each task available to run in the queue.
		nHosts = distroQueueInfo.LengthWithDependenciesMet - len(provisioningHosts)
	} else {
		hostAllocator := scheduler.GetHostAllocator(config.Scheduler.HostAllocator)

		// nHosts is the number of additional hosts desired.
		// nHostsFree is the sum of the number of hosts that are currently free and the
		// number of hosts estimated to soon be free.
		nHosts, nHostsFree, err = hostAllocator(ctx, &hostAllocatorData)
		if err != nil {
			j.AddError(errors.Wrapf(err, "calculating the number of new hosts required for distro '%s'", j.DistroID))
			return
		}
	}

	grip.Info(message.Fields{
		"runner":             hostAllocatorJobName,
		"distro":             j.DistroID,
		"single_task_distro": distro.SingleTaskDistro,
		"operation":          "runtime-stats",
		"phase":              "host-allocation",
		"instance":           j.ID(),
		"duration_secs":      time.Since(hostAllocationBegins).Seconds(),
	})

	//////////////////////
	// host-spawning phase
	//////////////////////

	numIntentHosts, err := host.CountIntentHosts(ctx)
	grip.Error(message.WrapError(err, message.Fields{
		"runner":   hostAllocatorJobName,
		"instance": j.ID(),
		"distro":   j.DistroID,
		"message":  "failed to count intent hosts",
	}))

	if numIntentHosts > maxIntentHosts {
		grip.Info(message.Fields{
			"runner":    hostAllocatorJobName,
			"instance":  j.ID(),
			"distro":    j.DistroID,
			"message":   "too many intent hosts, skipping host allocation",
			"max_hosts": maxIntentHosts,
		})
		return
	}

	hostSpawningBegins := time.Now()
	// Number of new hosts to be allocated
	hostsSpawned, err := scheduler.SpawnHosts(ctx, *distro, nHosts, containerPool)
	if err != nil {
		j.AddError(errors.Wrap(err, "spawning new hosts"))
		return
	}

	grip.Info(message.Fields{
		"runner":             hostAllocatorJobName,
		"distro":             distro.Id,
		"single_task_distro": distro.SingleTaskDistro,
		"operation":          "runtime-stats",
		"phase":              "host-spawning",
		"instance":           j.ID(),
		"duration_secs":      time.Since(hostSpawningBegins).Seconds(),
	})

	if err := EnqueueHostCreateJobs(ctx, j.env, hostsSpawned); err != nil {
		j.AddError(errors.Wrapf(err, "enqueueing host create jobs"))
	}

	// ignoring all the tasks that will take longer than the threshold to run,
	// and the hosts allocated for them,
	// how long will it take the current fleet of hosts, plus the ones we spawned, to chew through
	// the scheduled tasks in the queue?

	// The number of task group tasks that have been waiting >= MaxDurationThreshold since their dependencies were met
	var totalOverdueInTaskGroups int
	// The number of task group tasks have their dependencies met and are expected to take over MaxDurationThreshold
	var countDurationOverThresholdInTaskGroups int
	// The total number of hosts that are dedicated to running task groups that are free or are estimated to soon be free
	var freeInTaskGroups int
	// The total number of hosts that are dedicated to running task groups that are running tasks
	var requiredInTaskGroups int
	// The sum of the expected durations of all task group tasks that have their dependencies met and are expected to take over MaxDurationThreshold
	var durationOverThresholdInTaskGroups time.Duration
	// The sum of the expected durations of all task group tasks that have their dependencies met
	var expectedDurationInTaskGroups time.Duration

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

	// The sum of the expected durations of all standalone tasks that have their dependencies met
	correctedExpectedDuration := distroQueueInfo.ExpectedDuration - expectedDurationInTaskGroups
	// The sum of the expected durations of all standalone tasks that have their dependencies met and are expected to take over MaxDurationThreshold
	correctedDurationOverThreshold := distroQueueInfo.DurationOverThreshold - durationOverThresholdInTaskGroups
	// The sum of the expected durations of all standalone tasks that have their dependencies met and are expected to take under MaxDurationThreshold
	scheduledDuration := correctedExpectedDuration - correctedDurationOverThreshold
	// The number of standalone tasks that have their dependencies met and are expected to take over MaxDurationThreshold
	durationOverThreshNoTaskGroups := distroQueueInfo.CountDurationOverThreshold - countDurationOverThresholdInTaskGroups

	// The number of additional hosts to be spawned that will be dedicated to standalone tasks
	correctedHostsSpawned := len(hostsSpawned) - requiredInTaskGroups
	// The number of hosts that are expected to be available for running standalone tasks that are expected to take under MaxDurationThreshold
	hostsAvail := (nHostsFree - freeInTaskGroups) + correctedHostsSpawned - durationOverThreshNoTaskGroups

	// The total amount of time the tasks with dependencies met and are expected to take under MaxDurationThreshold
	// in the queue will take to complete when ran on the number of hosts we expect to be available for them
	var timeToEmpty time.Duration
	// The total amount of time the tasks with dependencies met and are expected to take under MaxDurationThreshold
	// in the queue will take to complete when ran on the number of hosts if only free (or soon to be free) hosts are
	// used
	var timeToEmptyNoSpawns time.Duration

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

	// How many multiples of MaxDurationThreshold the timeToEmpty variable is
	hostQueueRatio := float32(timeToEmpty.Nanoseconds()) / float32(distroQueueInfo.MaxDurationThreshold.Nanoseconds())
	// How many multiples of MaxDurationThreshold the timeToEmptyNoSpawns variable is
	noSpawnsRatio := float32(timeToEmptyNoSpawns.Nanoseconds()) / float32(distroQueueInfo.MaxDurationThreshold.Nanoseconds())

	// rough value that should correspond to situations where a queue will be empty very soon
	const lowRatioThresh = float32(.25)
	terminationOn := distro.HostAllocatorSettings.HostsOverallocatedRule == evergreen.HostsOverallocatedTerminate
	terminatableDistro := utility.StringSliceContains(evergreen.ProviderSpawnable, distro.Provider)
	if terminationOn && terminatableDistro && hostQueueRatio < lowRatioThresh && len(upHosts) > 0 {
		distroIsByHour := cloud.UsesHourlyBilling(&upHosts[0].Distro)
		if !distroIsByHour {
			j.setTargetAndTerminate(ctx, len(upHosts), hostQueueRatio, distro)
		}
	}

	grip.Info(message.Fields{
		"message":                            "distro-scheduler-report",
		"job_type":                           hostAllocatorJobName,
		"distro":                             distro.Id,
		"single_task_distro":                 distro.SingleTaskDistro,
		"provider":                           distro.Provider,
		"max_hosts":                          distro.HostAllocatorSettings.MaximumHosts,
		"num_new_hosts":                      len(hostsSpawned),
		"pool_info":                          existingHosts.Stats(),
		"task_queue_length":                  distroQueueInfo.Length,
		"task_queue_length_dependencies_met": distroQueueInfo.LengthWithDependenciesMet,
		"num_hosts_running":                  len(upHosts),
		"num_hosts_provisioning":             len(provisioningHosts),
		"merge_queue_tasks":                  distroQueueInfo.CountDepFilledMergeQueueTasks,
		"overdue_tasks":                      distroQueueInfo.CountWaitOverThreshold,
		"overdue_tasks_in_groups":            totalOverdueInTaskGroups,
		"total_runtime":                      distroQueueInfo.ExpectedDuration.String(),
		"runtime_secs":                       distroQueueInfo.ExpectedDuration.Seconds(),
		"time_to_empty":                      timeToEmpty.String(),
		"time_to_empty_mins":                 timeToEmpty.Minutes(),
		"time_to_empty_no_spawns":            timeToEmptyNoSpawns.String(),
		"time_to_empty_no_spawns_mins":       timeToEmptyNoSpawns.Minutes(),
		"host_queue_ratio":                   hostQueueRatio,
		"host_queue_ratio_no_spawns":         noSpawnsRatio,
		"instance":                           j.ID(),
		"runner":                             scheduler.RunnerName,
	})

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String(evergreen.DistroIDOtelAttribute, distro.Id),
		attribute.String(fmt.Sprintf("%s.distro_provider", hostAllocatorAttributePrefix), distro.Provider),
		attribute.Int(fmt.Sprintf("%s.distro_max_hosts", hostAllocatorAttributePrefix), distro.HostAllocatorSettings.MaximumHosts),
		attribute.Bool(fmt.Sprintf("%s.single_task_distro", hostAllocatorAttributePrefix), distro.SingleTaskDistro),
		attribute.Int(fmt.Sprintf("%s.hosts_requested", hostAllocatorAttributePrefix), len(hostsSpawned)),
		attribute.Int(fmt.Sprintf("%s.hosts_free", hostAllocatorAttributePrefix), nHostsFree),
		attribute.Int(fmt.Sprintf("%s.hosts_running", hostAllocatorAttributePrefix), len(upHosts)),
		attribute.Int(fmt.Sprintf("%s.hosts_total", hostAllocatorAttributePrefix), existingHosts.Stats().Total),
		attribute.Int(fmt.Sprintf("%s.hosts_active", hostAllocatorAttributePrefix), existingHosts.Stats().Active),
		attribute.Int(fmt.Sprintf("%s.hosts_idle", hostAllocatorAttributePrefix), existingHosts.Stats().Idle),
		attribute.Int(fmt.Sprintf("%s.hosts_provisioning", hostAllocatorAttributePrefix), existingHosts.Stats().Provisioning),
		attribute.Int(fmt.Sprintf("%s.hosts_quarantined", hostAllocatorAttributePrefix), existingHosts.Stats().Quarantined),
		attribute.Int(fmt.Sprintf("%s.hosts_decommissioned", hostAllocatorAttributePrefix), existingHosts.Stats().Decommissioned),
		attribute.Int(fmt.Sprintf("%s.task_queue_length", hostAllocatorAttributePrefix), distroQueueInfo.Length),
		attribute.Int(fmt.Sprintf("%s.overdue_tasks", hostAllocatorAttributePrefix), distroQueueInfo.CountWaitOverThreshold),
		attribute.Int(fmt.Sprintf("%s.merge_queue_tasks", hostAllocatorAttributePrefix), distroQueueInfo.CountDepFilledMergeQueueTasks),
		attribute.Int(fmt.Sprintf("%s.overdue_tasks_in_groups", hostAllocatorAttributePrefix), totalOverdueInTaskGroups),
		attribute.Float64(fmt.Sprintf("%s.queue_ratio", hostAllocatorAttributePrefix), float64(noSpawnsRatio)),
		attribute.Float64(fmt.Sprintf("%s.host_queue_ratio", hostAllocatorAttributePrefix), float64(hostQueueRatio)),
		attribute.Float64(fmt.Sprintf("%s.runtime_secs", hostAllocatorAttributePrefix), distroQueueInfo.ExpectedDuration.Seconds()),
		attribute.Float64(fmt.Sprintf("%s.time_to_empty_secs", hostAllocatorAttributePrefix), timeToEmpty.Seconds()),
		attribute.Float64(fmt.Sprintf("%s.time_to_empty_no_spawns_secs", hostAllocatorAttributePrefix), timeToEmptyNoSpawns.Seconds()),
	)
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
				"message":  "could not enqueue job to draw down hosts",
				"instance": j.ID(),
				"distro":   distro.Id,
			}))
		}

	}

}

// saveHostStats saves the latest host usage stats for an EC2 distro.
func (j *hostAllocatorJob) saveHostStats(ctx context.Context, d *distro.Distro, numUpHosts int) {
	if !evergreen.IsEc2Provider(d.Provider) {
		return
	}

	hs := hoststat.NewHostStat(d.Id, numUpHosts)
	if err := hs.Insert(ctx); err != nil && !db.IsDuplicateKey(err) {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "could not insert latest host stat data for distro",
			"distro":    d.Id,
			"num_hosts": numUpHosts,
		}))
	}

}
