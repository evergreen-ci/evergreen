package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
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
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteMinuteJobName, util.RoundPartOfMinute(0).Format(tsFormat)))
	return j
}

func (j *cronsRemoteMinuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := []amboy.QueueOperation{
		PopulateHostCreationJobs(j.env, 0),
		PopulateIdleHostJobs(j.env),
		PopulateHostTerminationJobs(j.env),
		PopulateHostMonitoring(j.env),
		PopulateEventAlertProcessing(1),
		PopulateBackgroundStatsJobs(j.env, 0),
		PopulateLastContainerFinishTimeJobs(),
		PopulateParentDecommissionJobs(),
		PopulatePeriodicNotificationJobs(1),
		PopulateContainerStateJobs(j.env),
		PopulateOldestImageRemovalJobs(),
		PopulateCommitQueueJobs(j.env),
	}

	queue := j.env.RemoteQueue()

	catcher := grip.NewBasicCatcher()
	for _, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}

		catcher.Add(op(queue))
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
