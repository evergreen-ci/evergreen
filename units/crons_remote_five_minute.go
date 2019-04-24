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

const cronsRemoteFiveMinuteJobName = "crons-remote-five-minute"

func init() {
	registry.AddJobType(cronsRemoteFiveMinuteJobName, NewCronRemoteFiveMinuteJob)
}

type cronsRemoteFiveMinuteJob struct {
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
	ErrorCount int `bson:"error_count" json:"error_count" yaml:"error_count"`
	env        evergreen.Environment
}

func NewCronRemoteFiveMinuteJob() amboy.Job {
	j := &cronsRemoteFiveMinuteJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cronsRemoteFiveMinuteJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteFiveMinuteJobName, util.RoundPartOfHour(5).Format(tsFormat)))
	return j
}

func (j *cronsRemoteFiveMinuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := []amboy.QueueOperation{
		PopulateTaskMonitoring(5),
		PopulateActivationJobs(5),
		PopulateRepotrackerPollingJobs(5),
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
		"queue": "service",
		"id":    j.ID(),
		"type":  j.Type().Name,
		"num":   len(ops),
		"errs":  j.ErrorCount,
	})
}
