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

const cronsRemoteFifteenSecondJobName = "crons-remote-fifteen-second"

func init() {
	registry.AddJobType(cronsRemoteFifteenSecondJobName, NewCronRemoteFifteenSecondJob)
}

type cronsRemoteFifteenSecondJob struct {
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
	ErrorCount int `bson:"error_count" json:"error_count" yaml:"error_count"`
	env        evergreen.Environment
}

func NewCronRemoteFifteenSecondJob() amboy.Job {
	j := &cronsRemoteFifteenSecondJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cronsRemoteFifteenSecondJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteFifteenSecondJobName, util.RoundPartOfMinute(15).Format(tsFormat)))
	return j
}

func (j *cronsRemoteFifteenSecondJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := []amboy.QueueOperation{
		PopulateHostSetupJobs(j.env),
		PopulateSchedulerJobs(j.env),
		PopulateAliasSchedulerJobs(j.env),
		PopulateHostAllocatorJobs(j.env),
		PopulateAgentDeployJobs(j.env),
		PopulateAgentMonitorDeployJobs(j.env),
		PopulateUserDataDoneJobs(j.env),
	}

	queue := j.env.RemoteQueue()

	catcher := grip.NewBasicCatcher()
	for _, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}

		catcher.Add(op(ctx, queue))
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
