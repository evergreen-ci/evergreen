package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const cronsRemoteMinuteJobName = "crons-remote-minute"

func init() {
	registry.AddJobType(cronsRemoteMinuteJobName, NewCronRemoteMinuteJob)
}

type cronsRemoteMinuteJob struct {
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
	ErrorCount int `bson:"error_count" json:"error_count" yaml:"error_count"`
	env        evergreen.Environment
}

func NewCronRemoteMinuteJob() amboy.Job {
	j := &cronsRemoteMinuteJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cronsRemoteMinuteJobName,
				Version: 0,
			},
		},
	}
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteMinuteJobName, utility.RoundPartOfMinute(0).Format(TSFormat)))
	return j
}

func (j *cronsRemoteMinuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := map[string]cronJobFactory{
		"host ready":                 hostReadyJob,
		"background stats":           backgroundStatsJobs,
		"container state":            containerStateJobs,
		"event send":                 sendNotificationJobs,
		"host monitoring":            hostMonitoringJobs,
		"last container finish time": lastContainerFinishTimeJobs,
		"oldest image removal":       oldestImageRemovalJobs,
		"parent decommission":        parentDecommissionJobs,
		"periodic notification":      periodicNotificationJobs,
		"user data done":             userDataDoneJobs,
	}

	var allJobs []amboy.Job
	catcher := grip.NewBasicCatcher()
	ts := utility.RoundPartOfMinute(0)

	for name, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}
		jobs, err := op(ctx, j.env, ts)
		if err != nil {
			catcher.Wrapf(err, "getting '%s' jobs", name)
			continue
		}
		allJobs = append(allJobs, jobs...)
	}
	catcher.Wrap(amboy.EnqueueManyUniqueJobs(ctx, j.env.RemoteQueue(), allJobs), "populating main queue")
	catcher.Add(enqueueHostSetupJobs(ctx, j.env, j.env.RemoteQueue(), ts))

	catcher.Add(populateQueueGroup(ctx, j.env, createHostQueueGroup, hostCreationJobs, ts))
	catcher.Add(populateQueueGroup(ctx, j.env, eventNotifierQueueGroup, eventNotifierJobs, ts))
	catcher.Add(populateQueueGroup(ctx, j.env, spawnHostModificationQueueGroup, sleepSchedulerJobs, ts))
	catcher.Add(populateQueueGroup(ctx, j.env, terminateHostQueueGroup, hostTerminationJobs, ts))

	// Add generate tasks fallbacks to their versions' queues.
	catcher.Add(enqueueFallbackGenerateTasksJobs(ctx, j.env, ts))

	j.ErrorCount = catcher.Len()

	grip.Debug(message.Fields{
		"id":    j.ID(),
		"type":  j.Type().Name,
		"queue": "service",
		"num":   len(ops),
		"errs":  j.ErrorCount,
	})
}
