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

	ops := []amboy.QueueOperation{
		PopulateSchedulerJobs(j.env),
		PopulateAliasSchedulerJobs(j.env),
		PopulateHostAllocatorJobs(j.env),
		PopulateAgentDeployJobs(j.env),
		PopulateAgentMonitorDeployJobs(j.env),
	}

	queue := j.env.RemoteQueue()

	catcher := grip.NewBasicCatcher()
	for _, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}

		catcher.Add(op(ctx, queue))
	}

	// Create dedicated queue for pod allocation.
	appCtx, _ := j.env.Context()
	podAllocationQueue, err := j.env.RemoteQueueGroup().Get(appCtx, podAllocationQueueGroup)
	if err != nil {
		catcher.Wrap(err, "getting pod allocator queue")
	} else {
		catcher.Wrap(PopulatePodAllocatorJobs(j.env)(ctx, podAllocationQueue), "populating pod allocator jobs")
	}

	podDefCreationQueue, err := j.env.RemoteQueueGroup().Get(appCtx, podDefinitionCreationQueueGroup)
	if err != nil {
		catcher.Wrap(err, "getting pod creation queue")
	} else {
		catcher.Wrap(PopulatePodDefinitionCreationJobs(j.env)(ctx, podDefCreationQueue), "populating pod definition creation jobs")
	}
	podCreationQueue, err := j.env.RemoteQueueGroup().Get(appCtx, podCreationQueueGroup)
	if err != nil {
		catcher.Wrap(err, "getting pod creation queue")
	} else {
		catcher.Wrap(PopulatePodCreationJobs()(ctx, podCreationQueue), "populating pod creation jobs")
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
