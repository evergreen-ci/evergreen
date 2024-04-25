package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const sleepSchedulerJobName = "sleep-scheduler"

func init() {
	registry.AddJobType(sleepSchedulerJobName, func() amboy.Job {
		return makeSleepSchedulerJob()
	})
}

type sleepSchedulerJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewSleepSchedulerJob creates a job to manage unexpirable host sleep
// schedules.
func NewSleepSchedulerJob(env evergreen.Environment, ts string) amboy.Job {
	j := makeSleepSchedulerJob()
	j.SetID(fmt.Sprintf("%s.%s", sleepSchedulerJobName, ts))
	j.SetEnqueueScopes(sleepSchedulerJobName)
	j.env = env
	return j
}

func makeSleepSchedulerJob() *sleepSchedulerJob {
	j := &sleepSchedulerJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    sleepSchedulerJobName,
				Version: 1,
			},
		},
	}
	return j
}

func (j *sleepSchedulerJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	j.AddError(errors.Wrap(j.syncPermanentlyExemptHosts(ctx), "syncing permanently exempt hosts"))
	j.AddError(errors.Wrap(j.fixMissingNextScheduleTimes(ctx), "fixing hosts that are missing next scheduled start/stop times"))
	j.AddError(errors.Wrap(j.fixHostsExceedingTimeout(ctx), "fixing hosts that are exceeding the scheduled stop/start timeout"))

	ts := utility.RoundPartOfMinute(0)
	j.AddError(errors.Wrap(populateQueueGroup(ctx, j.env, spawnHostModificationQueueGroup, j.makeStopAndStartJobs, ts), "enqueueing stop and start jobs"))
}

func (j *sleepSchedulerJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	return nil
}

const sleepScheduleUser = "sleep_schedule"

// fixMissingNextScheduleTimes finds and fixes hosts that are subject to the
// sleep schedule but are missing next stop/start times. For example, a host
// that was kept off but is now back on should be put back on its regular sleep
// schedule.
func (j *sleepSchedulerJob) fixMissingNextScheduleTimes(ctx context.Context) error {
	hosts, err := host.FindMissingNextSleepScheduleTime(ctx)
	if err != nil {
		return errors.Wrap(err, "finding hosts with missing next stop/start times")
	}
	now := time.Now()
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if utility.IsZeroTime(h.SleepSchedule.NextStartTime) {
			oldNextStart := h.SleepSchedule.NextStartTime
			nextStart, err := h.SleepSchedule.GetNextScheduledStartTime(now)
			if err != nil {
				catcher.Wrapf(err, "getting next start time for host '%s'", h.Id)
				continue
			}
			if err := h.SetNextScheduledStart(ctx, nextStart); err != nil {
				catcher.Wrapf(err, "setting next start time for host '%s'", h.Id)
				continue
			}

			grip.Notice(message.Fields{
				"message":             "host is missing next start time, re-scheduled to next available start time",
				"host_id":             h.Id,
				"started_by":          h.StartedBy,
				"old_next_start_time": oldNextStart,
				"new_next_start_time": nextStart,
				"job":                 j.ID(),
			})
		}
		if utility.IsZeroTime(h.SleepSchedule.NextStopTime) {
			oldNextStop := h.SleepSchedule.NextStopTime
			nextStop, err := h.SleepSchedule.GetNextScheduledStopTime(now)
			if err != nil {
				catcher.Wrapf(err, "getting next stop time for host '%s'", h.Id)
				continue
			}
			if err := h.SetNextScheduledStop(ctx, nextStop); err != nil {
				catcher.Wrapf(err, "setting next stop time for host '%s'", h.Id)
				continue
			}

			grip.Notice(message.Fields{
				"message":            "host is missing next stop time, re-scheduled to next available stop time",
				"host_id":            h.Id,
				"started_by":         h.StartedBy,
				"old_next_stop_time": oldNextStop,
				"new_next_stop_time": nextStop,
				"job":                j.ID(),
			})
		}
	}
	return catcher.Resolve()
}

// fixHostsExceedingScheduledTimeout finds and reschedules the next stop/start
// time for hosts that need to stop/start for their sleep schedule but have
// taken too long while attempting to stop/start.
func (j *sleepSchedulerJob) fixHostsExceedingTimeout(ctx context.Context) error {
	hosts, err := host.FindExceedsSleepScheduleTimeout(ctx)
	if err != nil {
		return errors.Wrap(err, "finding hosts exceeding sleep schedule timeout")
	}
	now := time.Now()
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if now.Sub(h.SleepSchedule.NextStartTime) > host.SleepScheduleActionTimeout {
			oldNextStart := h.SleepSchedule.NextStartTime
			nextStart, err := h.SleepSchedule.GetNextScheduledStartTime(now)
			if err != nil {
				catcher.Wrapf(err, "getting next start time for host '%s'", h.Id)
				continue
			}
			if err := h.SetNextScheduledStart(ctx, nextStart); err != nil {
				catcher.Wrapf(err, "setting next start time for host '%s'", h.Id)
				continue
			}

			grip.Warning(message.Fields{
				"message":             "host has exceeded scheduled start timeout, re-scheduled to next available start time",
				"host_id":             h.Id,
				"started_by":          h.StartedBy,
				"job":                 j.ID(),
				"old_next_start_time": oldNextStart,
				"new_next_start_time": nextStart,
			})
		}
		if now.Sub(h.SleepSchedule.NextStopTime) > host.SleepScheduleActionTimeout {
			oldNextStop := h.SleepSchedule.NextStopTime
			nextStop, err := h.SleepSchedule.GetNextScheduledStopTime(now)
			if err != nil {
				catcher.Wrapf(err, "getting next stop time for host '%s'", h.Id)
				continue
			}
			if err := h.SetNextScheduledStop(ctx, nextStop); err != nil {
				catcher.Wrapf(err, "setting next stop time for host '%s'", h.Id)
				continue
			}

			grip.Warning(message.Fields{
				"message":            "host has exceeded scheduled stop timeout, re-scheduling to next available stop time",
				"host_id":            h.Id,
				"started_by":         h.StartedBy,
				"job":                j.ID(),
				"old_next_stop_time": oldNextStop,
				"new_next_stop_time": nextStop,
			})
		}
	}
	return catcher.Resolve()
}

// syncPermanentlyExemptHosts ensures that the hosts that are marked as
// permanently exempt are consistent with the most up-to-date list of
// permanently exempt hosts.
func (j *sleepSchedulerJob) syncPermanentlyExemptHosts(ctx context.Context) error {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	return host.SyncPermanentExemptions(ctx, settings.SleepSchedule.PermanentlyExemptHosts)
}

func (j *sleepSchedulerJob) makeStopAndStartJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "checking if sleep schedule is enabled")
	}

	var stopJobs []amboy.Job
	var hostIDsToStop []string
	if !flags.SleepScheduleDisabled {
		// If sleep schedules are disabled, disable just the auto-stopping of
		// hosts, not the auto-starting of hosts. The sleep schedule feature
		// might be toggled as sleep schedules are rolling out to users (e.g. in
		// case of issues). Even if the feature is turned off while their host
		// is stopped, we assume users would still prefer have their host be
		// turned back on during waking hours for their convenience.
		hostsToStop, err := host.FindHostsScheduledToStop(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "finding hosts to stop")
		}
		stopJobs = make([]amboy.Job, 0, len(hostsToStop))
		hostIDsToStop = make([]string, 0, len(hostsToStop))
		for i := range hostsToStop {
			h := hostsToStop[i]
			stopJobs = append(stopJobs, NewSpawnhostStopJob(&h, false, evergreen.ModifySpawnHostSleepSchedule, sleepScheduleUser, ts.Format(TSFormat)))
			hostIDsToStop = append(hostIDsToStop, h.Id)
		}
	}

	hostsToStart, err := host.FindHostsScheduledToStart(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts to start")
	}
	startJobs := make([]amboy.Job, 0, len(hostsToStart))
	hostIDsToStart := make([]string, 0, len(hostsToStart))
	for i := range hostsToStart {
		h := hostsToStart[i]
		startJobs = append(startJobs, NewSpawnhostStartJob(&h, evergreen.ModifySpawnHostSleepSchedule, sleepScheduleUser, ts.Format(TSFormat)))
		hostIDsToStart = append(hostIDsToStart, h.Id)
	}

	grip.InfoWhen(len(hostIDsToStop) > 0, message.Fields{
		"message":  "enqueueing jobs to stop hosts for sleep schedule",
		"num_jobs": len(hostIDsToStop),
		"host_ids": hostIDsToStop,
		"job":      j.ID(),
	})
	grip.InfoWhen(len(hostIDsToStart) > 0, message.Fields{
		"message":  "enqueueing jobs to start hosts for sleep schedule",
		"num_jobs": len(hostIDsToStart),
		"host_ids": hostIDsToStart,
		"job":      j.ID(),
	})

	return append(stopJobs, startJobs...), nil
}
