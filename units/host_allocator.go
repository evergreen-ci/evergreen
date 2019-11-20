package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/scheduler"
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
	job := makeHostAllocatorJob()
	job.DistroID = distroID
	job.SetID(fmt.Sprintf("%s.%s.%s", hostAllocatorJobName, distroID, timestamp.Format(tsFormat)))
	job.env = env

	return job
}

func (j *hostAllocatorJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()
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

	distro, err := distro.FindOne(distro.ById(j.DistroID))
	if err != nil {
		j.AddError(errors.Wrapf(err, "Database error for find() by distro id '%s'", j.DistroID))
		return
	}

	if err = scheduler.UpdateStaticDistro(distro); err != nil {
		j.AddError(errors.Wrap(err, "problem updating static hosts"))
		return
	}

	var containerPool *evergreen.ContainerPool
	if distro.ContainerPool != "" {
		containerPool = settings.ContainerPools.GetContainerPool(distro.ContainerPool)
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
		Distro:           distro,
		ExistingHosts:    upHosts,
		FreeHostFraction: config.Scheduler.FreeHostFraction,
		UsesContainers:   (containerPool != nil),
		ContainerPool:    containerPool,
		DistroQueueInfo:  distroQueueInfo,
	}

	// nHosts is the number of additional hosts desired.
	nHosts, err := hostAllocator(ctx, hostAllocatorData)
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
	hostsSpawned, err := scheduler.SpawnHosts(ctx, distro, nHosts, containerPool)
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

	var makespan time.Duration
	if len(hostsSpawned) != 0 {
		makespan = distroQueueInfo.ExpectedDuration / time.Duration(len(hostsSpawned))
	} else if distroQueueInfo.Length > 0 {
		makespan = distroQueueInfo.ExpectedDuration
	}

	grip.Info(message.Fields{
		"message":                 "distro-scheduler-report",
		"job_type":                hostAllocatorJobName,
		"distro":                  distro.Id,
		"provider":                distro.Provider,
		"max_hosts":               distro.HostAllocatorSettings.MaximumHosts,
		"num_new_hosts":           len(hostsSpawned),
		"pool_info":               existingHosts.Stats(),
		"queue":                   eventInfo,
		"total_runtime":           distroQueueInfo.ExpectedDuration.String(),
		"runtime_secs":            distroQueueInfo.ExpectedDuration.Seconds(),
		"predicted_makespan":      makespan.String(),
		"predicted_makespan_secs": makespan.Seconds(),
		"instance":                j.ID(),
		"runner":                  scheduler.RunnerName,
	})
}
