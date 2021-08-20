package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
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
	j.SetDependency(dependency.NewAlways())
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteMinuteJobName, utility.RoundPartOfMinute(0).Format(TSFormat)))
	return j
}

func (j *cronsRemoteMinuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := []amboy.QueueOperation{
		PopulateHostSetupJobs(j.env),
		PopulateBackgroundStatsJobs(j.env, 0),
		PopulateContainerStateJobs(j.env),
		PopulateDataCleanupJobs(j.env),
		PopulateEventAlertProcessing(j.env),
		PopulateGenerateTasksJobs(j.env),
		PopulateHostMonitoring(j.env),
		PopulateHostTerminationJobs(j.env),
		PopulateIdleHostJobs(j.env),
		PopulateLastContainerFinishTimeJobs(),
		PopulateOldestImageRemovalJobs(),
		PopulateParentDecommissionJobs(),
		PopulatePeriodicNotificationJobs(1),
		PopulateUserDataDoneJobs(j.env),
		PopulatePodCreationJobs(j.env),
		PopulatePodTerminationJobs(j.env),
	}

	catcher := grip.NewBasicCatcher()

	queue := j.env.RemoteQueue()
	for _, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}
		catcher.Add(op(ctx, queue))
	}

	// Create dedicated queues for host creation and commit queue jobs
	appCtx, _ := j.env.Context()
	hcqueue, err := j.env.RemoteQueueGroup().Get(appCtx, "service.host.create")
	if err != nil {
		catcher.Add(errors.Wrap(err, "error getting host create queue"))
	} else {
		catcher.Add(PopulateHostCreationJobs(j.env, 0)(ctx, hcqueue))
	}

	commitQueueQueue, err := j.env.RemoteQueueGroup().Get(appCtx, "service.commitqueue")
	if err != nil {
		catcher.Add(errors.Wrap(err, "error getting commit queue queue"))
	} else {
		catcher.Add(PopulateCommitQueueJobs(j.env)(ctx, commitQueueQueue))
	}

	j.ErrorCount = catcher.Len()

	grip.Debug(message.Fields{
		"id":    j.ID(),
		"type":  j.Type().Name,
		"queue": "service",
		"num":   len(ops),
		"errs":  j.ErrorCount,
	})
}
