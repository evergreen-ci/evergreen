package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
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
	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}
}

func (j *sleepSchedulerJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	return nil
}
