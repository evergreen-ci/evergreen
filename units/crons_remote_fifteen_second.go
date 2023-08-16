package units

import (
	"context"
	"fmt"
	"time"

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

	ops := map[string]jobFactory{
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
		jobs, err := op(ctx, ts)
		if err != nil {
			catcher.Wrapf(err, "getting '%s' jobs", name)
			continue
		}
		allJobs = append(allJobs, jobs...)
	}
	catcher.Wrap(amboy.EnqueueManyUniqueJobs(ctx, j.env.RemoteQueue(), allJobs), "populating main queue")

	// Create dedicated queues for pod allocation.
	appCtx, _ := j.env.Context()
	catcher.Add(populateQueueGroup(appCtx, j.env, podAllocationQueueGroup, podAllocatorJobs, ts))
	catcher.Add(populateQueueGroup(appCtx, j.env, podDefinitionCreationQueueGroup, podDefinitionCreationJobs, ts))
	catcher.Add(populateQueueGroup(appCtx, j.env, podCreationQueueGroup, podCreationJobs, ts))

	j.ErrorCount = catcher.Len()
	grip.Error(message.WrapError(catcher.Resolve(), message.Fields{
		"id":    j.ID(),
		"type":  j.Type().Name,
		"queue": "service",
	}))
}

func populateQueueGroup(ctx context.Context, env evergreen.Environment, queueGroupName string, factory jobFactory, ts time.Time) error {
	queueGroup, err := env.RemoteQueueGroup().Get(ctx, queueGroupName)
	if err != nil {
		return errors.Wrapf(err, "getting '%s' queue", queueGroupName)
	}
	jobs, err := factory(ctx, ts)
	if err != nil {
		return errors.Wrapf(err, "getting '%s' jobs", queueGroupName)
	}

	return errors.Wrapf(amboy.EnqueueManyUniqueJobs(ctx, queueGroup, jobs), "populating '%s' queue", queueGroupName)
}
