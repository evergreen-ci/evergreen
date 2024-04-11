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
	j.env = env
	j.SetScopes([]string{sleepSchedulerJobName})
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

	ts := utility.RoundPartOfMinute(0)
	if err := populateQueueGroup(ctx, j.env, spawnHostModificationQueueGroup, j.makeStopAndStartJobs, ts); err != nil {
		j.AddError(errors.Wrap(err, "enqueuing stop and start jobs"))
		return
	}
}

func (j *sleepSchedulerJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	return nil
}

const sleepScheduleUser = "sleep_schedule"

// fixMissingNextScheduledTimes finds and fixes hosts that are subject to the
// sleep schedule but are missing next stop/start times.
func (j *sleepSchedulerJob) fixMissingNextScheduledTimes(ctx context.Context) error {
	hosts, err := host.FindMissingNextSleepScheduleTime(ctx)
	if err != nil {
		return errors.Wrap(err, "finding hosts with missing next stop/start times")
	}
	// now := time.Now()
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if utility.IsZeroTime(h.SleepSchedule.NextStartTime) {
			// nextStart, err := h.SleepSchedule.GetNextScheduledStartTime(now)
			// if err != nil {
			//     catcher.Wrapf(err, "getting next start time for host '%s'", h.Id)
			//     continue
			// }
			// // kim: TODO: needs DEVPROD-3951
			// h.SetNextScheduledStartTime(ctx, nextStart)
		}
		if utility.IsZeroTime(h.SleepSchedule.NextStopTime) {
			// nextStop, err := h.SleepSchedule.GetNextScheduledStopTime(now)
			// if err != nil {
			//     catcher.Wrapf(err, "getting next stop time for host '%s'", h.Id)
			//     continue
			// }
			// // kim: TODO: needs DEVPROD-3951
			// h.SetNextScheduledStopTime(ctx, nextStop)
		}
	}
	return catcher.Resolve()
}

// fixHostsExceedingScheduledTimeout finds and reschedules the next stop/start
// time for hosts that need to stop/start for their sleep schedule but have
// taken too long while attempting to stop/start.
func (j *sleepSchedulerJob) fixHostsExceedingScheduledTimeout(ctx context.Context) error {
	hosts, err := host.FindExceedsSleepScheduleActionTimeout(ctx)
	if err != nil {
		return errors.Wrap(err, "finding hosts exceeding scheduled threshold")
	}
	now := time.Now()
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if h.SleepSchedule.NextStopTime.Before(now.Add(-host.SleepScheduleActionTimeout)) {
			// nextStop, err := h.SleepSchedule.GetNextScheduledStopTime(now)
			// if err != nil {
			//     catcher.Wrapf(err, "getting next stop time for host '%s'", h.Id)
			//     continue
			// }
			// // kim: TODO: needs DEVPROD-3951
			// h.SetNextScheduledStopTime(ctx, nextStop)
		}
		if h.SleepSchedule.NextStartTime.Before(now.Add(-host.SleepScheduleActionTimeout)) {
			// nextStart, err := h.SleepSchedule.GetNextScheduledStartTime(now)
			// if err != nil {
			//     catcher.Wrapf(err, "getting next start time for host '%s'", h.Id)
			//     continue
			// }
			// // kim: TODO: needs DEVPROD-3951
			// h.SetNextScheduledStartTime(ctx, nextStart)
		}
	}
	return catcher.Resolve()
}

// syncPermanentlyExemptHosts ensures that the hosts that are marked as
// permanently exempt are consistent with the most up-to-date list of
// permanently exempt hosts.
func (j *sleepSchedulerJob) syncPermanentlyExemptHosts(ctx context.Context) error {
	settings := j.env.Settings()
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
