package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const schedulerJobName = "distro-scheduler"

func init() {
	registry.AddJobType(schedulerJobName, func() amboy.Job {
		return makeDistroSchedulerJob()
	})
}

type distroSchedulerJob struct {
	DistroID string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func makeDistroSchedulerJob() *distroSchedulerJob {
	j := &distroSchedulerJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    schedulerJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

func NewDistroSchedulerJob(distroID string, ts time.Time) amboy.Job {
	j := makeDistroSchedulerJob()
	j.DistroID = distroID
	j.SetID(fmt.Sprintf("%s.%s.%s", schedulerJobName, distroID, ts.Format(tsFormat)))

	return j
}

func (j *distroSchedulerJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	conf := scheduler.Configuration{
		DistroID:   j.DistroID,
		TaskFinder: "legacy",
	}

	err := scheduler.PlanDistro(ctx, conf)

	j.AddError(err)
}
