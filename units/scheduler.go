package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
	return j
}

func NewDistroSchedulerJob(distroID string, id string) amboy.Job {
	j := makeDistroSchedulerJob()
	j.DistroID = distroID
	j.SetID(fmt.Sprintf("%s.%s.%s", schedulerJobName, distroID, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", schedulerJobName, distroID)})
	j.SetEnqueueAllScopes(true)

	return j
}

func (j *distroSchedulerJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.SchedulerDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"distro":   j.DistroID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting scheduler settings"))
		return
	}
	conf := scheduler.Configuration{
		DistroID:   j.DistroID,
		TaskFinder: settings.Scheduler.TaskFinder,
	}

	j.AddError(scheduler.PlanDistro(ctx, conf, settings))
}
