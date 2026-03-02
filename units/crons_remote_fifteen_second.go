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
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteFifteenSecondJobName, utility.RoundPartOfMinute(15).Format(TSFormat)))
	return j
}

func (j *cronsRemoteFifteenSecondJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := map[string]cronJobFactory{
		"scheduler":            schedulerJobs,
		"alias scheduler":      aliasSchedulerJobs,
		"host allocator":       hostAllocatorJobs,
		"idle host":            idleHostJobs,
		"agent deploy":         agentDeployJobs,
		"agent monitor deploy": agentMonitorDeployJobs,
	}

	var allJobs []amboy.Job
	catcher := grip.NewBasicCatcher()
	ts := utility.RoundPartOfMinute(15)
	for name, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
			return
		}
		jobs, err := op(ctx, j.env, ts)
		if err != nil {
			catcher.Wrapf(err, "getting '%s' jobs", name)
			continue
		}
		allJobs = append(allJobs, jobs...)
	}
	catcher.Wrap(amboy.EnqueueManyUniqueJobs(ctx, j.env.RemoteQueue(), allJobs), "populating main queue")

	catcher.Add(populateQueueGroup(ctx, j.env, hostIPAssociationQueueGroup, hostIPAssociationJobs, ts))

	j.ErrorCount = catcher.Len()
	grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
		"id":    j.ID(),
		"type":  j.Type().Name,
		"queue": "service",
	}))
}
