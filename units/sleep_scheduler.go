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

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "checking if sleep schedule is enabled"))
		return
	}
	if flags.SleepScheduleDisabled {
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

func (j *sleepSchedulerJob) makeStopAndStartJobs(ctx context.Context, _ evergreen.Environment, ts time.Time) ([]amboy.Job, error) {
	hostsToStop, err := host.FindHostsScheduledToStop(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts to stop")
	}
	stopJobs := make([]amboy.Job, 0, len(hostsToStop))
	hostIDsToStop := make([]string, 0, len(hostsToStop))
	for i := range hostsToStop {
		h := hostsToStop[i]
		stopJobs = append(stopJobs, NewSpawnhostStopJob(&h, evergreen.User, ts.Format(TSFormat)))
		hostIDsToStop = append(hostIDsToStop, h.Id)
	}

	hostsToStart, err := host.FindHostsScheduledToStart(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts to start")
	}
	startJobs := make([]amboy.Job, 0, len(hostsToStart))
	hostIDsToStart := make([]string, 0, len(hostsToStart))
	for i := range hostsToStart {
		h := hostsToStart[i]
		startJobs = append(startJobs, NewSpawnhostStartJob(&h, evergreen.User, ts.Format(TSFormat)))
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
